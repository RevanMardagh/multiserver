package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var port int
	var bind string
	var maxClients int
	var logDir string

	flag.IntVar(&port, "port", 4444, "listen port")
	flag.IntVar(&port, "p", 4444, "listen port (shorthand)")
	flag.StringVar(&bind, "bind", "0.0.0.0", "bind address")
	flag.StringVar(&bind, "b", "0.0.0.0", "bind address (shorthand)")
	flag.IntVar(&maxClients, "max", 100, "max concurrent clients")
	flag.IntVar(&maxClients, "m", 100, "max concurrent clients (shorthand)")
	flag.StringVar(&logDir, "logdir", "logs", "log directory")
	flag.StringVar(&logDir, "l", "logs", "log directory (shorthand)")
	flag.Parse()

	if port < 0 || port > 65535 {
		fmt.Fprintf(os.Stderr, "[!] port must be 0-65535, got %d\n", port)
		os.Exit(1)
	}

	s := NewServer(bind, port, maxClients, logDir)

	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[*] listening on %s:%d  (max %d connections)\n", bind, port, maxClients)
	fmt.Printf("[*] commands: list, <id> <msg>, all <msg>, kick <id>, interact <id>, os <id> <windows|linux>, tag <id> <name>, alias, run <id> <alias>, help, exit\n")
	fmt.Printf("[*] logging per-client to ./%s/\n", logDir)

	// Handle OS signals for graceful shutdown (non-interactive Ctrl-C)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n[*] caught signal, shutting down...")
		s.Shutdown()
		s.closeRL()
		// Second signal forces immediate exit
		<-sigCh
		os.Exit(1)
	}()

	s.RunConsole()

	fmt.Println("[*] shutting down...")
	s.Shutdown()
	fmt.Println("[*] shut down")
}
