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

	s := NewServer(bind, port, maxClients, logDir)

	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[*] listening on %s:%d  (max %d connections)\n", bind, port, maxClients)
	fmt.Printf("[*] commands: list, <id> <msg>, all <msg>, kick <id>, interact <id>, os <id> <windows|linux>, alias, run <id> <alias>, help, exit\n")
	fmt.Printf("[*] logging per-client to ./%s/\n", logDir)

	// Handle OS signals for graceful shutdown (non-interactive Ctrl-C)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		// If in interactive mode, Ctrl-C is handled by readline, not here.
		// But the signal handler is a fallback for non-readline contexts.
		fmt.Println("\n[*] caught signal, shutting down...")
		s.Shutdown()
		os.Exit(0)
	}()

	s.RunConsole()

	fmt.Println("[*] shutting down...")
	s.Shutdown()
	fmt.Println("[*] shut down")
}
