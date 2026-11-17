package main

import (
	"strings"
	"testing"
)

func TestSubstituteArgsSimple(t *testing.T) {
	result, err := substituteArgs("curl {0} -o {1}", []string{"http://example.com", "/tmp/file"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "curl http://example.com -o /tmp/file" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestSubstituteArgsNoPlaceholders(t *testing.T) {
	result, err := substituteArgs("whoami /all", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "whoami /all" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestSubstituteArgsTooFewArgs(t *testing.T) {
	_, err := substituteArgs("curl {0} -o {1}", []string{"http://example.com"})
	if err == nil {
		t.Fatal("expected error for too few args")
	}
}

func TestSubstituteArgsLiteralBraces(t *testing.T) {
	result, err := substituteArgs("echo hello", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "echo hello" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestCountFields(t *testing.T) {
	tests := []struct {
		cmd  string
		want int
	}{
		{"curl {0} -o {1}", 2},
		{"whoami /all", 0},
		{"{0}", 1},
		{"{0} {0}", 1}, // same placeholder used twice = 1 unique field
	}
	for _, tt := range tests {
		got := countFields(tt.cmd)
		if got != tt.want {
			t.Errorf("countFields(%q) = %d, want %d", tt.cmd, got, tt.want)
		}
	}
}

func TestSubstituteArgsZeroArgsWhenNeeded(t *testing.T) {
	_, err := substituteArgs("curl -fsSL {0} -o {1}", nil)
	if err == nil {
		t.Fatal("expected error for zero args when needed")
	}
	// Error message should mention the counts.
	if !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "0") {
		t.Errorf("error should mention 2 and 0: %v", err)
	}
}

func TestDefaultAliasCount(t *testing.T) {
	aliases := defaultAliases()
	// Python version has 28 aliases.
	if len(aliases) != 28 {
		t.Errorf("expected 28 default aliases, got %d", len(aliases))
	}
}

func TestDefaultAliasesHaveBothOS(t *testing.T) {
	aliases := defaultAliases()
	for name, a := range aliases {
		if a.Windows == "" {
			t.Errorf("alias %q missing Windows command", name)
		}
		if a.Linux == "" {
			t.Errorf("alias %q missing Linux command", name)
		}
	}
}
