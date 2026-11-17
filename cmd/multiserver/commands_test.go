package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestDispatchExit(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if s.Dispatch("exit") != false {
		t.Fatal("expected exit to return false")
	}
	if s.Dispatch("quit") != false {
		t.Fatal("expected quit to return false")
	}
}

func TestDispatchHelp(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if s.Dispatch("help") != true {
		t.Fatal("expected help to return true")
	}
	if s.Dispatch("?") != true {
		t.Fatal("expected ? to return true")
	}
}

func TestDispatchUnknown(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if s.Dispatch("garbage") != true {
		t.Fatal("expected unknown command to return true")
	}
}

func TestDispatchSendToClient(t *testing.T) {
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
		t.Fatal("no clients")
	}
	cid := clients[0].ID

	s.Dispatch(fmt.Sprintf("%d whoami", cid))

	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "whoami\n" {
		t.Fatalf("expected %q, got %q", "whoami\n", string(buf[:n]))
	}
}

func TestDispatchAll(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn1, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()

	conn2, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	time.Sleep(200 * time.Millisecond)

	s.Dispatch("all hello")

	for i, conn := range []net.Conn{conn1, conn2} {
		buf := make([]byte, 64)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("conn%d read error: %v", i+1, err)
		}
		if string(buf[:n]) != "hello\n" {
			t.Fatalf("conn%d: expected %q, got %q", i+1, "hello\n", string(buf[:n]))
		}
	}
}

func TestDispatchOsSet(t *testing.T) {
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
		t.Fatal("no clients")
	}
	cid := clients[0].ID

	s.Dispatch(fmt.Sprintf("os %d linux", cid))
	client := s.GetClient(cid)
	if client == nil {
		t.Fatal("client not found")
	}
	if client.OS != "linux" {
		t.Fatalf("expected OS 'linux', got %q", client.OS)
	}
}

func TestDispatchRunAlias(t *testing.T) {
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
		t.Fatal("no clients")
	}
	cid := clients[0].ID

	s.Dispatch(fmt.Sprintf("os %d linux", cid))
	s.Dispatch(fmt.Sprintf("run %d whoami", cid))

	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	// "whoami" alias on linux = "id"
	if string(buf[:n]) != "id\n" {
		t.Fatalf("expected %q, got %q", "id\n", string(buf[:n]))
	}
}

func TestDispatchTildeEscapeNormalMode(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	// Not in interactive mode — ~. should be treated as unknown command
	result := s.Dispatch("~.")
	if result != true {
		t.Fatal("expected ~. to return true (not quit)")
	}
}

func TestDispatchKick(t *testing.T) {
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
	cid := clients[0].ID

	s.Dispatch(fmt.Sprintf("kick %d", cid))
	time.Sleep(500 * time.Millisecond)

	clients = s.ListClients()
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients after kick, got %d", len(clients))
	}
}

func TestDispatchEmptyLine(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if s.Dispatch("") != true {
		t.Fatal("expected empty line to return true")
	}
	if s.Dispatch("   ") != true {
		t.Fatal("expected whitespace-only line to return true")
	}
}

func TestDispatchInteract(t *testing.T) {
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
	cid := clients[0].ID

	s.Dispatch(fmt.Sprintf("interact %d", cid))
	if s.InteractCID() != int32(cid) {
		t.Fatalf("expected interactCID %d, got %d", cid, s.InteractCID())
	}
}
