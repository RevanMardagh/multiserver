package main

import (
	"net"
	"testing"
	"time"
)

func TestServerAcceptsConnection(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	time.Sleep(200 * time.Millisecond)

	clients := s.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].ID != 1 {
		t.Fatalf("expected client ID 1, got %d", clients[0].ID)
	}
}

func TestServerRejectsAtMaxClients(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 1, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn1, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()
	time.Sleep(200 * time.Millisecond)

	conn2, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	time.Sleep(200 * time.Millisecond)

	clients := s.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client (max reached), got %d", len(clients))
	}
}

func TestServerSendTo(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(200 * time.Millisecond)

	clients := s.ListClients()
	if len(clients) == 0 {
		t.Fatal("no clients connected")
	}

	if err := s.SendTo(clients[0].ID, "hello"); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "hello\n" {
		t.Fatalf("expected %q, got %q", "hello\n", string(buf[:n]))
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	s.Shutdown()

	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected error after shutdown, got nil")
	}
}

func TestServerPortInUse(t *testing.T) {
	s1 := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s1.Start(); err != nil {
		t.Fatal(err)
	}
	defer s1.Shutdown()

	port := s1.Addr().(*net.TCPAddr).Port
	s2 := NewServer("127.0.0.1", port, 10, t.TempDir())
	err := s2.Start()
	if err == nil {
		s2.Shutdown()
		t.Fatal("expected error for port in use")
	}
}

func TestServerInteractCID(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if s.InteractCID() != -1 {
		t.Fatalf("expected initial interactCID -1, got %d", s.InteractCID())
	}
	s.SetInteractCID(42)
	if s.InteractCID() != 42 {
		t.Fatalf("expected interactCID 42, got %d", s.InteractCID())
	}
	s.SetInteractCID(-1)
	if s.InteractCID() != -1 {
		t.Fatalf("expected interactCID -1, got %d", s.InteractCID())
	}
}

func TestServerInvalidUTF8(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(200 * time.Millisecond)

	// Send invalid UTF-8 bytes — should not panic
	conn.Write([]byte{0xff, 0xfe, 0x68, 0x69, 0x0a}) // invalid + "hi\n"
	time.Sleep(200 * time.Millisecond)

	// If we got here without a panic, the test passes
	clients := s.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
}
