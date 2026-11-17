package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chzyer/readline"
)

// RunConsole starts the interactive readline loop. It blocks until the
// user types exit/quit or the server shuts down. Returns nil on clean exit.
func (s *Server) RunConsole() error {
	historyFile := ""
	if home, err := os.UserHomeDir(); err == nil {
		historyFile = filepath.Join(home, ".multiserver_history")
	}

	if historyFile != "" {
		if f, err := os.OpenFile(historyFile, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			f.Close()
		}
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:                 "> ",
		HistoryFile:            historyFile,
		AutoComplete:           s.DynamicCompleter(),
		InterruptPrompt:        "^C",
		EOFPrompt:              "",
		DisableAutoSaveHistory: true,
	})
	if err != nil {
		return fmt.Errorf("readline init: %w", err)
	}
	defer s.closeRL()

	s.rlMu.Lock()
	s.rl = rl
	s.rlMu.Unlock()

	for s.running.Load() {
		// Update prompt based on interactive mode
		cid := s.InteractCID()
		if cid >= 0 {
			rl.SetPrompt(fmt.Sprintf("[%d]$ ", cid))
		} else {
			rl.SetPrompt("> ")
		}

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				// Ctrl-C: if interactive, exit interactive mode
				cid := s.InteractCID()
				if cid >= 0 {
					s.SetInteractCID(-1)
					fmt.Printf("[*] left interactive mode (client %d)\n", cid)
				}
				continue
			}
			if err == io.EOF {
				// Ctrl-D: ignore, only exit/quit shuts down
				continue
			}
			// Other error
			continue
		}

		// Interactive mode: raw passthrough
		cid = s.InteractCID()
		if cid >= 0 {
			if strings.TrimSpace(line) == "~." {
				s.SetInteractCID(-1)
				fmt.Printf("[*] left interactive mode (client %d)\n", cid)
				continue
			}
			if err := s.SendTo(int(cid), line); err != nil {
				fmt.Printf("[!] %v\n", err)
			}
			continue
		}

		// Normal mode: dispatch command
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rl.SaveHistory(line)
		if !s.Dispatch(line) {
			break
		}
	}

	return nil
}

// DynamicCompleter returns a readline completer that includes static
// commands plus dynamically-generated client IDs and alias names.
func (s *Server) DynamicCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("list"),
		readline.PcItem("ls"),
		readline.PcItem("all"),
		readline.PcItem("kick",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("interact",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("i",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("os",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("download",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("upload",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("alias",
			readline.PcItem("add"),
			readline.PcItem("del",
				readline.PcItemDynamic(s.completeAliasNames),
			),
		),
		readline.PcItem("run",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("tag",
			readline.PcItemDynamic(s.completeClientIDs),
		),
		readline.PcItem("group",
			readline.PcItem("add"),
			readline.PcItem("del",
				readline.PcItemDynamic(s.completeGroupNames),
			),
			readline.PcItem("show",
				readline.PcItemDynamic(s.completeGroupNames),
			),
		),
		readline.PcItem("gr",
			readline.PcItemDynamic(s.completeGroupNames),
		),
		readline.PcItem("grun",
			readline.PcItemDynamic(s.completeGroupNames),
		),
		readline.PcItem("help"),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
		readline.PcItem("?"),
	)
}

// completeClientIDs returns the currently connected client IDs as strings.
func (s *Server) completeClientIDs(line string) []string {
	clients := s.ListClients()
	ids := make([]string, len(clients))
	for i, c := range clients {
		ids[i] = fmt.Sprintf("%d", c.ID)
	}
	return ids
}

// completeAliasNames returns the current alias names sorted.
func (s *Server) completeAliasNames(line string) []string {
	s.mu.RLock()
	names := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	return names
}

// completeGroupNames returns the current group names sorted.
func (s *Server) completeGroupNames(line string) []string {
	s.mu.RLock()
	names := make([]string, 0, len(s.groups))
	for name := range s.groups {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	return names
}
