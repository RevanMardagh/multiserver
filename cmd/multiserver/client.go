package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Client represents a connected reverse shell client.
type Client struct {
	ID   int
	Conn net.Conn
	Addr net.Addr
	OS   string // "windows", "linux", or ""
	Log  *ClientLogger
}

// recvLoop reads data from the client connection in a loop.
// It runs as a goroutine and exits when the connection closes
// or the server shuts down.
func (c *Client) recvLoop(s *Server) {
	defer s.wg.Done()
	defer s.RemoveClient(c.ID)

	buf := make([]byte, 4096)
	for s.running.Load() {
		c.Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := c.Conn.Read(buf)
		if n > 0 {
			raw := buf[:n]
			text := strings.ToValidUTF8(string(raw), "�")

			if c.Log != nil {
				c.Log.Log("<", text)
			}

			if s.InteractCID() == int32(c.ID) {
				// Interactive mode: write raw bytes to stdout
				os.Stdout.Write(raw)
			} else {
				// Normal mode: formatted output
				fmt.Printf("[%s] [%d] %s\n", ts(), c.ID, strings.TrimRight(text, "\r\n"))
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Connection closed or error
			break
		}
	}
}

// ts returns the current time as HH:MM:SS for console output.
func ts() string {
	return time.Now().Format("15:04:05")
}
