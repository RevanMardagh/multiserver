package main

import (
	"flag"
	"fmt"
	"os"
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

	fmt.Fprintf(os.Stderr, "[*] config: bind=%s port=%d max=%d logdir=%s\n", bind, port, maxClients, logDir)
}
