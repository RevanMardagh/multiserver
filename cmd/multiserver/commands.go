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
	"tag":      cmdTag,
	"group":    cmdGroup,
	"gr":       cmdGroupRun,
	"grun":     cmdGroupRunAlias,
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

	// Fallback: <id|tag> <message>
	split2 := strings.SplitN(line, " ", 2)
	if len(split2) == 2 {
		client := resolveClient(s, split2[0])
		if client != nil {
			if err := s.SendTo(client.ID, split2[1]); err != nil {
				fmt.Printf("[!] %v\n", err)
			}
			return true
		}
	}

	fmt.Println("[!] usage: <id> <message>   (try 'list')")
	return true
}

// resolveClient finds a client by numeric ID or by tag name (case-insensitive).
// Returns nil if no match is found.
func resolveClient(s *Server, idOrTag string) *Client {
	if cid, err := strconv.Atoi(idOrTag); err == nil {
		return s.GetClient(cid)
	}
	lower := strings.ToLower(idOrTag)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.clients {
		if strings.ToLower(c.Tag) == lower {
			return c
		}
	}
	return nil
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
		if c.Tag != "" {
			fmt.Printf("    %d: %s  [%s]  (%s)\n", c.ID, c.Addr, osFlag, c.Tag)
		} else {
			fmt.Printf("    %d: %s  [%s]\n", c.ID, c.Addr, osFlag)
		}
	}
	return true
}

func cmdKick(s *Server, args []string) bool {
	if len(args) < 2 {
		fmt.Println("[!] usage: kick <id|tag>")
		return true
	}
	client := resolveClient(s, args[1])
	if client == nil {
		fmt.Printf("[!] no client %q\n", args[1])
		return true
	}
	client.Conn.Close()
	return true
}

func cmdInteract(s *Server, args []string) bool {
	if len(args) < 2 {
		fmt.Println("[!] usage: interact <id|tag>")
		return true
	}
	client := resolveClient(s, args[1])
	if client == nil {
		fmt.Printf("[!] no client %q\n", args[1])
		return true
	}
	s.SetInteractCID(int32(client.ID))
	fmt.Printf("[*] interactive with client %d. '~.' or Ctrl-C to exit.\n", client.ID)
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
		client := resolveClient(s, args[1])
		if client == nil {
			fmt.Printf("[!] no client %q\n", args[1])
			return true
		}
		s.mu.Lock()
		if c, ok := s.clients[client.ID]; ok {
			c.OS = args[2]
		}
		s.mu.Unlock()
		fmt.Printf("[*] client %d flagged %s\n", client.ID, args[2])
		return true
	}
	fmt.Println("[!] usage: os <id> <windows|linux>")
	return true
}

func cmdAlias(s *Server, args []string) bool {
	if len(args) == 1 {
		s.mu.RLock()
		names := make([]string, 0, len(s.aliases))
		for name := range s.aliases {
			names = append(names, name)
		}
		if len(names) == 0 {
			s.mu.RUnlock()
			fmt.Println("    (no aliases)")
			return true
		}
		sort.Strings(names)
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
		fmt.Println("[!] usage: run <id|tag> <alias> [arg0] [arg1] ...")
		return true
	}
	client := resolveClient(s, args[1])
	if client == nil {
		fmt.Printf("[!] no client %q\n", args[1])
		return true
	}
	cid := client.ID
	aliasName := args[2]
	aliasArgs := args[3:]

	s.mu.RLock()
	alias, exists := s.aliases[aliasName]
	s.mu.RUnlock()

	if !exists {
		fmt.Printf("[!] no alias '%s' (try 'alias')\n", aliasName)
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

func cmdTag(s *Server, args []string) bool {
	if len(args) == 1 {
		clients := s.ListClients()
		tagged := false
		for _, c := range clients {
			if c.Tag != "" {
				fmt.Printf("    %d: %s\n", c.ID, c.Tag)
				tagged = true
			}
		}
		if !tagged {
			fmt.Println("    (no tags set)")
		}
		return true
	}
	client := resolveClient(s, args[1])
	if client == nil {
		fmt.Printf("[!] no client %q\n", args[1])
		return true
	}
	if len(args) == 2 {
		s.mu.Lock()
		if c, ok := s.clients[client.ID]; ok {
			c.Tag = ""
		}
		s.mu.Unlock()
		fmt.Printf("[*] client %d tag cleared\n", client.ID)
		return true
	}
	tag := args[2]
	s.mu.Lock()
	if c, ok := s.clients[client.ID]; ok {
		c.Tag = tag
	}
	s.mu.Unlock()
	fmt.Printf("[*] client %d tagged as %q\n", client.ID, tag)
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

func cmdGroup(s *Server, args []string) bool {
	if len(args) == 1 {
		// List all groups
		s.mu.RLock()
		if len(s.groups) == 0 {
			s.mu.RUnlock()
			fmt.Println("    (no groups)")
			return true
		}
		names := make([]string, 0, len(s.groups))
		for name := range s.groups {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			members := s.groups[name]
			ids := make([]int, 0, len(members))
			for id := range members {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			idStrs := make([]string, len(ids))
			for i, id := range ids {
				idStrs[i] = strconv.Itoa(id)
			}
			fmt.Printf("    %s: %d member(s) [%s]\n", name, len(members), strings.Join(idStrs, ", "))
		}
		s.mu.RUnlock()
		return true
	}

	sub := args[1]

	if sub == "add" {
		if len(args) < 4 {
			fmt.Println("[!] usage: group add <name> <id> [id...]")
			return true
		}
		name := args[2]
		s.mu.Lock()
		if s.groups[name] == nil {
			s.groups[name] = make(map[int]bool)
		}
		for _, idStr := range args[3:] {
			cid, err := strconv.Atoi(idStr)
			if err != nil {
				fmt.Printf("[!] invalid id '%s', skipping\n", idStr)
				continue
			}
			if _, exists := s.clients[cid]; !exists {
				fmt.Printf("[!] warning: client %d not connected\n", cid)
			}
			s.groups[name][cid] = true
		}
		s.mu.Unlock()
		fmt.Printf("[*] group '%s' updated\n", name)
		return true
	}

	if sub == "del" {
		if len(args) < 3 {
			fmt.Println("[!] usage: group del <name> [id]")
			return true
		}
		name := args[2]
		s.mu.Lock()
		members, exists := s.groups[name]
		if !exists {
			s.mu.Unlock()
			fmt.Printf("[!] no group '%s'\n", name)
			return true
		}
		if len(args) == 3 {
			// Delete entire group
			delete(s.groups, name)
			s.mu.Unlock()
			fmt.Printf("[*] group '%s' deleted\n", name)
			return true
		}
		// Remove one member
		cid, err := strconv.Atoi(args[3])
		if err != nil {
			s.mu.Unlock()
			fmt.Println("[!] usage: group del <name> [id]")
			return true
		}
		delete(members, cid)
		s.mu.Unlock()
		fmt.Printf("[*] client %d removed from group '%s'\n", cid, name)
		return true
	}

	if sub == "show" {
		if len(args) != 3 {
			fmt.Println("[!] usage: group show <name>")
			return true
		}
		name := args[2]
		s.mu.RLock()
		members, exists := s.groups[name]
		if !exists {
			s.mu.RUnlock()
			fmt.Printf("[!] no group '%s'\n", name)
			return true
		}
		if len(members) == 0 {
			s.mu.RUnlock()
			fmt.Printf("    group '%s': (empty)\n", name)
			return true
		}
		ids := make([]int, 0, len(members))
		for id := range members {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			client := s.clients[id]
			if client != nil {
				fmt.Printf("    %d: %s\n", id, client.Addr)
			} else {
				fmt.Printf("    %d: (disconnected)\n", id)
			}
		}
		s.mu.RUnlock()
		return true
	}

	fmt.Println("[!] usage: group | group add <name> <id> [id...] | group del <name> [id] | group show <name>")
	return true
}

func cmdGroupRun(s *Server, args []string) bool {
	if len(args) < 3 {
		fmt.Println("[!] usage: gr <group> <message>")
		return true
	}
	name := args[1]
	msg := strings.Join(args[2:], " ")

	s.mu.RLock()
	members, exists := s.groups[name]
	if !exists {
		s.mu.RUnlock()
		fmt.Printf("[!] no group '%s'\n", name)
		return true
	}
	ids := make([]int, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	sort.Ints(ids)
	for _, cid := range ids {
		if err := s.SendTo(cid, msg); err != nil {
			fmt.Printf("[!] %v\n", err)
		}
	}
	return true
}

func cmdGroupRunAlias(s *Server, args []string) bool {
	if len(args) < 3 {
		fmt.Println("[!] usage: grun <group> <alias> [arg0] [arg1] ...")
		return true
	}
	groupName := args[1]
	aliasName := args[2]
	aliasArgs := args[3:]

	s.mu.RLock()
	members, exists := s.groups[groupName]
	if !exists {
		s.mu.RUnlock()
		fmt.Printf("[!] no group '%s'\n", groupName)
		return true
	}
	alias, aliasExists := s.aliases[aliasName]
	ids := make([]int, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	if !aliasExists {
		fmt.Printf("[!] no alias '%s' (try 'alias')\n", aliasName)
		return true
	}

	sort.Ints(ids)
	for _, cid := range ids {
		client := s.GetClient(cid)
		if client == nil {
			fmt.Printf("[!] client %d not connected, skipping\n", cid)
			continue
		}
		if client.OS == "" {
			fmt.Printf("[!] client %d has no OS flag, skipping\n", cid)
			continue
		}

		var cmd string
		if client.OS == "windows" {
			cmd = alias.Windows
		} else {
			cmd = alias.Linux
		}

		if cmd == "" {
			fmt.Printf("[!] alias '%s' has no %s command, skipping client %d\n", aliasName, client.OS, cid)
			continue
		}

		resolved, err := substituteArgs(cmd, aliasArgs)
		if err != nil {
			needed := countFields(cmd)
			fmt.Printf("[!] alias '%s' needs %d arg(s)\n", aliasName, needed)
			return true
		}

		fmt.Printf("[*] alias '%s' -> client %d (%s): %s\n", aliasName, cid, client.OS, resolved)
		if err := s.SendTo(cid, resolved); err != nil {
			fmt.Printf("[!] %v\n", err)
		}
	}
	return true
}

const helpText = `  list / ls                         show connected clients
  <id|tag> <message>                send message to client
  all <message>                     broadcast to all clients
  kick <id|tag>                     disconnect client
  interact <id|tag> / i <id|tag>    raw passthrough mode (Ctrl-C or ~. to exit)
  os                                list OS flags
  os <id|tag> <windows|linux>       set client OS flag
  tag                               list tagged clients
  tag <id|tag> <name>               set client tag
  tag <id|tag>                      clear client tag
  alias                             list aliases
  alias add <name> <win> ::: <lin>  define alias
  alias del <name>                  remove alias
  run <id|tag> <name> [args]        run alias on client ({0}/{1} substituted)
  group                             list groups
  group add <name> <id> [id...]     add clients to group
  group del <name> [id]             delete group or remove member
  group show <name>                 show group members
  gr <group> <message>              send message to group
  grun <group> <alias> [args...]    run alias on group
  exit / quit                       shut down server
`
