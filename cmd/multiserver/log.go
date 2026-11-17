package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ClientLogger manages a per-client log file with its own mutex.
type ClientLogger struct {
	mu   sync.Mutex
	file *os.File
}

var newlineReplacer = strings.NewReplacer("\n", "\\n", "\r", "\\r")

// NewClientLogger creates a log file for a client. The file is named
// client_{cid}_{ip}_{port}.log inside dir. It writes an initial
// "connected" line and returns the logger ready for use.
func NewClientLogger(dir string, cid int, ip string, port int) (*ClientLogger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	fname := filepath.Join(dir, fmt.Sprintf("client_%d_%s_%d.log", cid, ip, port))
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	l := &ClientLogger{file: f}
	l.writeLine("*", fmt.Sprintf("connected from %s:%d", ip, port))
	return l, nil
}

// Log writes one or more lines (splitting on newlines) to the log file.
// Each line is prefixed with a timestamp and the given direction string.
func (l *ClientLogger) Log(direction string, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	safe := sanitizeForLog(newlineReplacer.Replace(text))
	if safe == "" {
		return
	}
	fmt.Fprintf(l.file, "[%s] %s %s\n", timestamp(), direction, safe)
	l.file.Sync()
}

// Close writes a disconnect line and closes the underlying file.
func (l *ClientLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLine("*", "disconnected")
	return l.file.Close()
}

// writeLine writes a single formatted log line. Caller must hold mu.
func (l *ClientLogger) writeLine(direction string, text string) {
	fmt.Fprintf(l.file, "[%s] %s %s\n", timestamp(), direction, text)
	l.file.Sync()
}

// sanitizeForLog strips control characters that could exploit terminal
// emulators when log files are viewed with cat/less/tail.
func sanitizeForLog(s string) string {
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
		return r
	}, s)
}

// timestamp returns the current time formatted as YYYY-MM-DD HH:MM:SS.
func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
