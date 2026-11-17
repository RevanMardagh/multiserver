package main

import (
	"net"
	"os"
	"strings"
	"time"
	"unicode"
)

// Client represents a connected reverse shell client.
type Client struct {
	ID       int
	Conn     net.Conn
	Addr     net.Addr
	OS       string // "windows", "linux", or ""
	Tag      string
	Log      *ClientLogger
	detected bool
}

// recvLoop reads data from the client connection in a loop.
// It runs as a goroutine and exits when the connection closes
// or the server shuts down.
func (c *Client) recvLoop(s *Server) {
	defer s.wg.Done()
	defer s.RemoveClient(c.ID)

	buf := make([]byte, 4096)
	for s.running.Load() {
		c.Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := c.Conn.Read(buf)
		if n > 0 {
			raw := buf[:n]
			text := strings.ToValidUTF8(string(raw), "�")

			if !c.detected {
				c.detected = true
				if c.OS == "" {
					if osName := detectOS(text); osName != "" {
						s.mu.Lock()
						if cl, ok := s.clients[c.ID]; ok {
							cl.OS = osName
						}
						s.mu.Unlock()
						s.safePrintf("[*] client %d auto-detected as %s\n", c.ID, osName)
					}
				}
			}

			if c.Log != nil {
				c.Log.Log("<", text)
			}

			if s.InteractCID() == int32(c.ID) {
				// Interactive mode: write raw bytes to stdout
				os.Stdout.Write(raw)
			} else {
				// Normal mode: formatted output with control chars stripped
				safe := sanitizeForTerminal(strings.TrimRight(text, "\r\n"))
				s.safePrintf("[%s] [%d] %s\n", ts(), c.ID, safe)
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Connection closed or error
			break
		}
	}
}

// sanitizeForTerminal strips control characters that could manipulate
// the operator's terminal (ANSI escapes, etc.). Preserves printable
// text, tabs, and spaces.
func sanitizeForTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7F {
			return -1
		}
		if r >= 0x80 && r <= 0x9F {
			return -1
		}
		if r == 0x2028 || r == 0x2029 {
			return -1
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
}

// ts returns the current time as HH:MM:SS for console output.
func ts() string {
	return time.Now().Format("15:04:05")
}

// detectOS examines shell prompt text and returns "windows", "linux",
// or "" if the OS cannot be determined.
func detectOS(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" {
			continue
		}

		// Windows indicators
		if strings.Contains(trimmed, `C:\`) || strings.Contains(trimmed, `D:\`) {
			return "windows"
		}
		if strings.HasPrefix(trimmed, "PS ") || strings.HasPrefix(trimmed, "PS>") {
			return "windows"
		}
		if len(trimmed) >= 3 && trimmed[1] == ':' && trimmed[2] == '\\' &&
			((trimmed[0] >= 'A' && trimmed[0] <= 'Z') || (trimmed[0] >= 'a' && trimmed[0] <= 'z')) {
			return "windows"
		}

		// Linux indicators
		if strings.HasSuffix(trimmed, "$") || strings.HasSuffix(trimmed, "#") {
			return "linux"
		}
		if strings.Contains(trimmed, "/home/") || strings.Contains(trimmed, "/root/") {
			return "linux"
		}
		if strings.Contains(trimmed, "bash") || strings.Contains(trimmed, "sh-") {
			return "linux"
		}
	}
	return ""
}
