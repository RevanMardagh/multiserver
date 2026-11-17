package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestClientLoggerWritesFormattedLines(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewClientLogger(dir, 1, "10.0.0.5", 54321)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	logger.Log("<", "hello world")
	logger.Log(">", "whoami")

	fname := filepath.Join(dir, "client_1_10.0.0.5_54321.log")
	data, err := os.ReadFile(fname)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// First line is the "connected" line, then our two log lines
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %q", len(lines), string(data))
	}

	pattern := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] `)

	// Check connected line
	if !pattern.MatchString(lines[0]) || !strings.Contains(lines[0], "* connected from 10.0.0.5:54321") {
		t.Errorf("unexpected connected line: %q", lines[0])
	}

	// Check data lines
	if !pattern.MatchString(lines[1]) || !strings.Contains(lines[1], "< hello world") {
		t.Errorf("unexpected log line 1: %q", lines[1])
	}
	if !pattern.MatchString(lines[2]) || !strings.Contains(lines[2], "> whoami") {
		t.Errorf("unexpected log line 2: %q", lines[2])
	}
}

func TestClientLoggerCreatesFileWithCorrectName(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewClientLogger(dir, 3, "192.168.1.1", 9999)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	fname := filepath.Join(dir, "client_3_192.168.1.1_9999.log")
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		t.Fatalf("expected file %s to exist", fname)
	}
}

func TestClientLoggerHandlesMultilineText(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewClientLogger(dir, 1, "10.0.0.1", 1234)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	logger.Log("<", "line1\nline2\nline3")

	fname := filepath.Join(dir, "client_1_10.0.0.1_1234.log")
	data, err := os.ReadFile(fname)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// 1 connected line + 3 data lines = 4
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), string(data))
	}

	pattern := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] < line\d$`)
	for _, line := range lines[1:] {
		if !pattern.MatchString(line) {
			t.Errorf("unexpected line format: %q", line)
		}
	}
}
