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
	ID      int
	Conn    net.Conn
	Addr    net.Addr
	OS      string // "windows", "linux", or ""
	Log     *ClientLogger
	Capture *CaptureState // non-nil during file download capture
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

			if c.Log != nil {
				c.Log.Log("<", text)
			}

			if c.Capture != nil {
				c.Capture.Process(text, s, c.ID)
				if c.Capture != nil && c.Capture.IsDone() {
					c.Capture = nil
				}
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
