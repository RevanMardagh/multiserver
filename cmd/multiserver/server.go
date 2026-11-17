package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chzyer/readline"
)

// Server manages the TCP listener, connected clients, and aliases.
type Server struct {
	mu           sync.RWMutex
	clients      map[int]*Client
	nextID       int
	interactCID  atomic.Int32 // -1 = none, else client ID
	aliases      map[string]*Alias
	logDir       string
	maxClients   int
	listener     net.Listener
	running      atomic.Bool
	shutdownOnce sync.Once
	wg           sync.WaitGroup
	bind         string
	port         int
	rl           *readline.Instance
}

// NewServer creates a new Server with the given configuration.
// Call Start() to begin listening.
func NewServer(bind string, port, maxClients int, logDir string) *Server {
	s := &Server{
		clients:    make(map[int]*Client),
		nextID:     1,
		aliases:    defaultAliases(),
		logDir:     logDir,
		maxClients: maxClients,
		bind:       bind,
		port:       port,
	}
	s.interactCID.Store(-1)
	return s
}

// Start binds the listener and launches the accept loop goroutine.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.bind, s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.listener = ln
	s.running.Store(true)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Addr returns the listener's address (useful when port 0 is used).
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Shutdown performs a graceful shutdown: stops accepting, closes all
// connections, waits for goroutines to finish.
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.running.Store(false)
		if s.listener != nil {
			s.listener.Close()
		}
		s.mu.Lock()
		conns := make([]net.Conn, 0, len(s.clients))
		for _, c := range s.clients {
			conns = append(conns, c.Conn)
		}
		s.mu.Unlock()
		for _, conn := range conns {
			conn.Close()
		}
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
}

// safePrintf writes formatted output through readline's safe writer
// when active, falling back to os.Stdout otherwise. Use from background
// goroutines (acceptLoop, recvLoop, RemoveClient) to avoid corrupting
// the readline prompt.
func (s *Server) safePrintf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if s.rl != nil {
		io.WriteString(s.rl.Stdout(), msg)
	} else {
		io.WriteString(os.Stdout, msg)
	}
}

// acceptLoop runs in its own goroutine, accepting new connections
// until the listener is closed.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for s.running.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running.Load() {
				return // Shutdown called
			}
			continue
		}

		s.mu.Lock()
		if len(s.clients) >= s.maxClients {
			s.mu.Unlock()
			addr := conn.RemoteAddr()
			s.safePrintf("[!] connection limit (%d) reached, rejecting %s\n", s.maxClients, addr)
			conn.Close()
			continue
		}

		cid := s.nextID
		s.nextID++

		tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		ip := tcpAddr.IP.String()
		port := tcpAddr.Port

		logger, err := NewClientLogger(s.logDir, cid, ip, port)
		if err != nil {
			s.safePrintf("[!] could not open log for client %d: %v\n", cid, err)
		}

		client := &Client{
			ID:   cid,
			Conn: conn,
			Addr: conn.RemoteAddr(),
			Log:  logger,
		}
		s.clients[cid] = client
		s.mu.Unlock()

		s.safePrintf("[%s] [*] client %d connected from %s:%d\n", ts(), cid, ip, port)

		s.wg.Add(1)
		go client.recvLoop(s)
	}
}

// SendTo sends a message to the client with the given ID.
func (s *Server) SendTo(cid int, msg string) error {
	s.mu.RLock()
	client := s.clients[cid]
	s.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("no client with id %d", cid)
	}

	_, err := client.Conn.Write([]byte(msg + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send to %d: %w", cid, err)
	}

	if client.Log != nil {
		client.Log.Log(">", msg)
	}
	return nil
}

// GetClient returns the client with the given ID, or nil.
func (s *Server) GetClient(cid int) *Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[cid]
}

// ListClients returns a snapshot of all connected clients, sorted by ID.
func (s *Server) ListClients() []*Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Client, 0, len(s.clients))
	for _, c := range s.clients {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// RemoveClient removes a client from the map, closes its log, and
// prints a disconnect message.
func (s *Server) RemoveClient(cid int) {
	s.mu.Lock()
	client, ok := s.clients[cid]
	if ok {
		delete(s.clients, cid)
	}
	s.mu.Unlock()

	if !ok {
		return
	}

	wasInteractive := s.InteractCID() == int32(cid)
	if wasInteractive {
		s.SetInteractCID(-1)
		s.safePrintf("\n[*] interacted client %d gone, back to console\n", cid)
	}

	if client.Log != nil {
		client.Log.Close()
	}
	client.Conn.Close()

	addr := client.Addr
	s.safePrintf("[%s] [*] client %d %s disconnected\n", ts(), cid, addr)
}

// InteractCID returns the current interactive mode client ID (-1 = none).
func (s *Server) InteractCID() int32 {
	return s.interactCID.Load()
}

// SetInteractCID sets the interactive mode client ID (-1 = none).
func (s *Server) SetInteractCID(cid int32) {
	s.interactCID.Store(cid)
}
