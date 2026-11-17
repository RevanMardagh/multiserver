package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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
	"download": cmdDownload,
	"upload":   cmdUpload,
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

func cmdDownload(s *Server, args []string) bool {
	if len(args) < 3 {
		fmt.Println("[!] usage: download <id> <remote_path>")
		return true
	}
	cid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("[!] usage: download <id> <remote_path>")
		return true
	}
	remotePath := strings.Join(args[2:], " ")

	client := s.GetClient(cid)
	if client == nil {
		fmt.Printf("[!] no client with id %d\n", cid)
		return true
	}
	if client.OS == "" {
		fmt.Printf("[!] client %d has no OS flag; use 'os %d windows|linux'\n", cid, cid)
		return true
	}
	if client.Capture != nil && !client.Capture.IsDone() {
		fmt.Printf("[!] client %d already has a download in progress\n", cid)
		return true
	}

	marker := randomHex(6)
	startMarker := "===DLSTART_" + marker + "==="
	endMarker := "===DLEND_" + marker + "==="

	localFile := filepath.Base(remotePath)
	// Strip any path separators that might remain (Windows backslash)
	localFile = filepath.Base(strings.ReplaceAll(remotePath, "\\", "/"))

	capture := &CaptureState{
		startMarker: startMarker,
		endMarker:   endMarker,
		localPath:   localFile,
		done:        make(chan struct{}),
	}

	s.mu.Lock()
	if c, ok := s.clients[cid]; ok {
		c.Capture = capture
	}
	s.mu.Unlock()

	cmd := buildDownloadCommand(client.OS, remotePath, startMarker, endMarker)
	if err := s.SendTo(cid, cmd); err != nil {
		fmt.Printf("[!] %v\n", err)
		s.mu.Lock()
		if c, ok := s.clients[cid]; ok {
			c.Capture = nil
		}
		s.mu.Unlock()
		return true
	}

	fmt.Printf("[*] download started: sent base64 command to client %d\n", cid)
	fmt.Printf("[*] file will be saved as: %s\n", localFile)
	return true
}

func cmdUpload(s *Server, args []string) bool {
	if len(args) < 4 {
		fmt.Println("[!] usage: upload <id> <local_path> <remote_path>")
		return true
	}
	cid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("[!] usage: upload <id> <local_path> <remote_path>")
		return true
	}
	localPath := args[2]
	remotePath := strings.Join(args[3:], " ")

	client := s.GetClient(cid)
	if client == nil {
		fmt.Printf("[!] no client with id %d\n", cid)
		return true
	}
	if client.OS == "" {
		fmt.Printf("[!] client %d has no OS flag; use 'os %d windows|linux'\n", cid, cid)
		return true
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		fmt.Printf("[!] cannot read local file: %v\n", err)
		return true
	}

	if len(data) > uploadMaxSize {
		fmt.Printf("[!] file too large (%d bytes, max %d bytes)\n", len(data), uploadMaxSize)
		return true
	}
	if len(data) > uploadWarnSize {
		fmt.Printf("[*] warning: large file (%d bytes), transfer may be slow\n", len(data))
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	cmds := buildUploadCommands(client.OS, remotePath, b64)

	fmt.Printf("[*] uploading %s (%d bytes, %d chunk(s)) to client %d\n", localPath, len(data), len(cmds), cid)
	for _, cmd := range cmds {
		if err := s.SendTo(cid, cmd); err != nil {
			fmt.Printf("[!] send failed: %v\n", err)
			return true
		}
	}
	fmt.Printf("[*] upload complete: sent to client %d -> %s\n", cid, remotePath)
	return true
}

const helpText = `  list / ls                         show connected clients
  <id> <message>                    send message to client
  all <message>                     broadcast to all clients
  kick <id>                         disconnect client
  interact <id> / i <id>            raw passthrough mode (Ctrl-C or ~. to exit)
  os                                list OS flags
  os <id> <windows|linux>           set client OS flag
  download <id> <remote_path>       download file from client (base64 capture)
  upload <id> <local> <remote>      upload local file to client
  alias                             list aliases
  alias add <name> <win> ::: <lin>  define alias
  alias del <name>                  remove alias
  run <id> <name> [arg0] [arg1] ... run alias on client ({0}/{1} substituted)
  exit / quit                       shut down server
`
