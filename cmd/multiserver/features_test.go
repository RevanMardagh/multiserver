package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestDetectOSWindows(t *testing.T) {
	cases := []string{
		"C:\\Users\\admin>",
		"PS C:\\Windows\\System32>",
		"PS> ",
		"D:\\Projects\\test>",
		"Microsoft Windows [Version 10.0.19045]\r\nC:\\Users\\admin>",
	}
	for _, input := range cases {
		got := detectOS(input)
		if got != "windows" {
			t.Errorf("detectOS(%q) = %q, want %q", input, got, "windows")
		}
	}
}

func TestDetectOSLinux(t *testing.T) {
	cases := []string{
		"user@host:~$",
		"root@host:/# ",
		"root@host:/root/test#",
		"/home/user",
		"bash-5.1$",
		"sh-5.0$",
		"user@host:~$ ",
	}
	for _, input := range cases {
		got := detectOS(input)
		if got != "linux" {
			t.Errorf("detectOS(%q) = %q, want %q", input, got, "linux")
		}
	}
}

func TestDetectOSUnknown(t *testing.T) {
	cases := []string{
		"",
		"hello world",
		"some random text",
	}
	for _, input := range cases {
		got := detectOS(input)
		if got != "" {
			t.Errorf("detectOS(%q) = %q, want %q", input, got, "")
		}
	}
}

func TestResolveClientByID(t *testing.T) {
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
	cid := clients[0].ID

	got := resolveClient(s, fmt.Sprintf("%d", cid))
	if got == nil {
		t.Fatal("resolveClient by ID returned nil")
	}
	if got.ID != cid {
		t.Fatalf("expected client ID %d, got %d", cid, got.ID)
	}
}

func TestResolveClientByTag(t *testing.T) {
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
	cid := clients[0].ID

	s.mu.Lock()
	s.clients[cid].Tag = "webserver"
	s.mu.Unlock()

	got := resolveClient(s, "webserver")
	if got == nil {
		t.Fatal("resolveClient by tag returned nil")
	}
	if got.ID != cid {
		t.Fatalf("expected client ID %d, got %d", cid, got.ID)
	}
}

func TestResolveClientByTagCaseInsensitive(t *testing.T) {
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

	s.mu.Lock()
	s.clients[cid].Tag = "WebServer"
	s.mu.Unlock()

	got := resolveClient(s, "WEBSERVER")
	if got == nil {
		t.Fatal("resolveClient by tag (case-insensitive) returned nil")
	}
	if got.ID != cid {
		t.Fatalf("expected client ID %d, got %d", cid, got.ID)
	}
}

func TestResolveClientNotFound(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	got := resolveClient(s, "999")
	if got != nil {
		t.Fatal("expected nil for nonexistent ID")
	}
	got = resolveClient(s, "nonexistent")
	if got != nil {
		t.Fatal("expected nil for nonexistent tag")
	}
}

func TestDispatchTagSetAndClear(t *testing.T) {
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

	s.Dispatch(fmt.Sprintf("tag %d webserver", cid))
	client := s.GetClient(cid)
	if client.Tag != "webserver" {
		t.Fatalf("expected tag 'webserver', got %q", client.Tag)
	}

	s.Dispatch(fmt.Sprintf("tag %d", cid))
	client = s.GetClient(cid)
	if client.Tag != "" {
		t.Fatalf("expected empty tag after clear, got %q", client.Tag)
	}
}

func TestReconnectTracking(t *testing.T) {
	s := NewServer("127.0.0.1", 0, 10, t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	conn1, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	clients := s.ListClients()
	if len(clients) == 0 {
		t.Fatal("no clients")
	}
	cid1 := clients[0].ID

	s.mu.Lock()
	s.clients[cid1].Tag = "mybox"
	s.clients[cid1].OS = "linux"
	s.mu.Unlock()

	s.mu.Lock()
	if s.groups["testgrp"] == nil {
		s.groups["testgrp"] = make(map[int]bool)
	}
	s.groups["testgrp"][cid1] = true
	s.mu.Unlock()

	conn1.Close()
	time.Sleep(1500 * time.Millisecond)

	conn2, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	time.Sleep(200 * time.Millisecond)

	clients = s.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	c := clients[0]

	if c.Tag != "mybox" {
		t.Errorf("expected tag 'mybox', got %q", c.Tag)
	}
	if c.OS != "linux" {
		t.Errorf("expected OS 'linux', got %q", c.OS)
	}

	s.mu.RLock()
	inGroup := s.groups["testgrp"][c.ID]
	s.mu.RUnlock()
	if !inGroup {
		t.Error("expected client to be restored to group 'testgrp'")
	}
}

func TestDispatchSendByTag(t *testing.T) {
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

	s.mu.Lock()
	s.clients[cid].Tag = "db"
	s.mu.Unlock()

	s.Dispatch("db whoami")

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
