# Go Rewrite Design — multiserver

## Intent

Rewrite multiserver.py in Go. Same feature set, better foundations: static binary (zero install), built-in race detector, goroutine concurrency, cross-compilation.

**Why now:** The Python version works but has a data race on `interact_cid`, thread-per-connection scaling limits, painful distribution (Python + pip required), and no tests. A rewrite fixes the structural problems instead of patching them.

## Goals

- Feature parity with the Python version (all commands, aliases, logging, interactive mode)
- Single static binary, cross-compilable (`GOOS`/`GOARCH`)
- One dependency: `github.com/chzyer/readline` (for editable prompt, history, tab completion, concurrent-safe output)
- Race-free by design (provable with `go run -race`)
- Testable: state in a struct, command dispatch decoupled from I/O

## Non-Goals

- TLS/auth (future work, not this rewrite)
- Plugin system or scripting
- GUI or web interface
- Rewriting aliases as Go functions (they stay as string templates sent over the wire)

## Anti-Goals

- Bloated binary with embedded web server or unnecessary deps
- Over-engineered architecture (this is a ~1000-line tool, not a framework)

## Constraints

- Must run on Windows, Linux, macOS
- Interactive mode must pass raw bytes (ANSI codes) through to the operator's terminal
- Graceful shutdown must flush all log files
- Existing README command table is the feature spec

## Approaches Considered

### A. Single-package, multi-file (chosen)

One Go module, one `main` package, split across files by concern:

```
cmd/multiserver/
  main.go       — entry point, flags, signal handling
  server.go     — Server struct, accept loop, client management
  client.go     — Client struct, recv goroutine
  console.go    — readline setup, command dispatch, interactive mode
  commands.go   — individual command handlers (list, kick, os, alias, run, etc.)
  aliases.go    — default alias table, alias CRUD, alias execution
  log.go        — per-client log file management
```

**Why:** Clean separation without over-engineering. Each file is 80-150 lines. All in one package so no export ceremony. `go build ./cmd/multiserver` produces the binary.

### B. Multi-package (`pkg/server`, `pkg/console`, etc.)

Split into importable packages.

**Rejected:** This isn't a library. Nobody imports `multiserver/pkg/server`. Internal packages add import paths and export rules for no consumer. Premature.

### C. Single file

**Rejected:** ~1000 lines of Go in one file is navigable but hurts testability and readability. The Python version's single-file design is one of its weaknesses.

## Architecture

### Core Struct

```go
type Server struct {
    mu          sync.RWMutex
    clients     map[int]*Client
    nextID      int
    interactCID atomic.Int32  // -1 = none, else client ID
    aliases     map[string]*Alias
    logDir      string
    maxClients  int
    listener    net.Listener
    running     atomic.Bool
}

type Client struct {
    ID     int
    Conn   net.Conn
    Addr   net.Addr
    OS     string // "windows", "linux", or ""
    Log    *ClientLogger
}

type Alias struct {
    Windows string
    Linux   string
}
```

`interactCID` uses `atomic.Int32` — no lock needed, no race possible. This directly fixes the #1 bug from the Python version.

### Concurrency Model

- **Accept loop**: one goroutine, runs until listener closed
- **Per-client recv**: one goroutine per connection, reads with 1s timeout, checks `running` between reads
- **Console**: main goroutine, readline blocks for input
- **Shutdown**: `running` set to false, listener closed (unblocks accept), all conns get `Close()` (unblocks recv), `sync.WaitGroup` waits for goroutine cleanup
- **Client map**: `sync.RWMutex` — reads (list, send) take RLock, writes (connect, disconnect) take Lock
- **Log writes**: each `ClientLogger` has its own `sync.Mutex` — no global log lock contention

### Command Dispatch

```go
type CmdHandler func(s *Server, args []string) bool // false = quit

var commands = map[string]CmdHandler{
    "list":     cmdList,
    "ls":       cmdList,
    "kick":     cmdKick,
    "interact": cmdInteract,
    "i":        cmdInteract,
    "os":       cmdOS,
    "alias":    cmdAlias,
    "run":      cmdRun,
    "all":      cmdAll,
    "help":     cmdHelp,
    "?":        cmdHelp,
    "exit":     cmdExit,
    "quit":     cmdExit,
}
```

Fallback: if first token parses as int, treat as `<id> <message>`. Otherwise, print usage.

### Alias Argument Substitution

Go's `fmt.Sprintf` doesn't support positional `{0}` syntax. Two options:
1. Use `strings.Replace` loop: `{0}` -> args[0], `{1}` -> args[1], etc.
2. Use `regexp.ReplaceAllStringFunc`

Option 1 is simpler and sufficient. Count fields by scanning for `{N}` patterns with a regex to validate arg count.

### Interactive Mode

- `interactCID` set via atomic store
- recv goroutine checks: if this client is interactive, write raw bytes to `os.Stdout` (bypassing readline)
- Console loop checks: if interactive, send raw line to client (no command dispatch)
- `~.` or Ctrl-C exits interactive mode (atomic store back to -1)
- readline instance gets swapped prompt: `[N]$ ` in interactive, `> ` in normal

### Per-Client Logging

```go
type ClientLogger struct {
    mu   sync.Mutex
    file *os.File
}
```

Each client owns its logger. No global log lock. Log format matches Python version:
```
[2026-11-17 16:30:00] * connected from 10.0.0.5:54321
[2026-11-17 16:30:01] > whoami
[2026-11-17 16:30:01] < root
[2026-11-17 16:30:05] * disconnected
```

### CLI Flags

```
multiserver [flags]
  -p, --port    int     listen port (default 4444)
  -b, --bind    string  bind address (default "0.0.0.0")
  -m, --max     int     max concurrent clients (default 100)
  -l, --logdir  string  log directory (default "logs")
```

Use stdlib `flag` package. No external CLI framework needed.

### Shutdown Sequence

1. `running` set to false
2. Close listener (unblocks accept goroutine)
3. Lock client map, copy all conns, unlock
4. Close each conn (unblocks recv goroutines)
5. `WaitGroup.Wait()` with timeout — all recv goroutines flush logs and exit
6. Close any remaining log files
7. Exit

### Connection Limits

Accept loop checks `len(clients) >= maxClients` before accepting. If at limit, accept the conn, send a rejection message, close it. This prevents the connection from hanging in the kernel backlog.

## File Estimates

| File | Lines | Responsibility |
|------|-------|---------------|
| main.go | ~60 | flags, signal handling, Server init, start |
| server.go | ~120 | Server struct, accept loop, shutdown, send_to |
| client.go | ~100 | Client struct, recv goroutine, ClientLogger |
| console.go | ~120 | readline setup, main input loop, interactive mode |
| commands.go | ~200 | all command handlers |
| aliases.go | ~150 | alias table, CRUD, run_alias with arg substitution |
| log.go | ~50 | ClientLogger implementation |
| **Total** | **~800** | |

## What's Left to the Builder

- Exact readline configuration (history file path, prompt colors)
- Whether to colorize `[*]` / `[!]` prefixes in output
- Tab completion: static command list vs. dynamic (include client IDs, alias names)
- Whether `alias add` syntax stays identical or gets improved
- Test file structure and scope (unit tests for commands + alias substitution at minimum)
