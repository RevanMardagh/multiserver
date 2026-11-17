# multiserver Go Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite multiserver from Python to Go — same features, single static binary, race-free concurrency.

**Architecture:** Single `main` package in `cmd/multiserver/`, split across 7 files by concern. `Server` struct owns all state. `sync.RWMutex` protects client map, `atomic.Int32` for interactive mode CID. One goroutine per client for recv, one for accept loop, main goroutine runs readline console. Graceful shutdown via `sync.WaitGroup`.

**Tech Stack:** Go 1.21+, `github.com/chzyer/readline`, stdlib only otherwise.

**Spec:** `docs/specs/2026-11-17-go-rewrite-design.md`

## Global Constraints

- Go 1.21+ (for `atomic.Int32`, `sync.OnceFunc`)
- One external dependency: `github.com/chzyer/readline`
- Must build on Windows, Linux, macOS without platform-specific code
- Must pass `go vet` and `go run -race` with zero findings
- All command names and alias names match Python version exactly
- Log format: `[YYYY-MM-DD HH:MM:SS] <direction> <text>` — identical to Python version
- Module path: `github.com/RevanMardagh/multiserver`

## Review Focus

1. **Client sends binary/non-UTF8 data in normal mode** — `string(data)` in Go is not lossy like Python `decode(errors="replace")`. Display must not panic; replace invalid bytes. Test: send `[]byte{0xff, 0xfe}`, verify output contains replacement chars.
2. **Concurrent disconnect during send_to** — `send_to` reads conn under RLock, sends outside lock. Client may disconnect between. Must handle write error without panic or dangling state. Test: close conn from another goroutine while `send_to` is in progress.
3. **`~.` typed when not in interactive mode** — should be treated as normal unknown command, not trigger exit-interact. Test: type `~.` at normal prompt, expect usage error.
4. **Alias with `{0}` but zero args supplied** — must print helpful error with arg count, not panic on index-out-of-range. Test: `run 1 dl` with no args.
5. **Port already in use** — `net.Listen` returns error. Must print clear message and exit, not panic. Test: bind same port twice.

---

## Prerequisite

Go must be installed. If not present: download from https://go.dev/dl/ and ensure `go` is in PATH. Verify with `go version` (need 1.21+).

---

### Task 1: Module scaffold + ClientLogger

**Files:**
- Create: `cmd/multiserver/main.go`
- Create: `cmd/multiserver/log.go`
- Create: `cmd/multiserver/log_test.go`
- Create: `go.mod`

**Interfaces:**
- Consumes: nothing (foundation task)
- Produces:
  - `ClientLogger` struct with methods:
    - `NewClientLogger(dir string, cid int, ip string, port int) (*ClientLogger, error)`
    - `(l *ClientLogger) Log(direction string, text string)`
    - `(l *ClientLogger) Close()`
  - `main()` stub that parses flags and prints listen address

- [ ] **Step 1: Initialize Go module**

Run:
```bash
mkdir -p cmd/multiserver
go mod init github.com/RevanMardagh/multiserver
```

- [ ] **Step 2: Write main.go stub with flag parsing**

Create `cmd/multiserver/main.go`. Parse flags using stdlib `flag`:
- `-port` / `-p`: int, default 4444
- `-bind` / `-b`: string, default `"0.0.0.0"`
- `-max` / `-m`: int, default 100
- `-logdir` / `-l`: string, default `"logs"`

Register both short and long forms via `flag.IntVar`/`flag.StringVar` pointing to same variable. Print parsed config and exit.

- [ ] **Step 3: Verify main.go builds and runs**

Run: `go build ./cmd/multiserver && ./multiserver -p 5555`
Expected: prints config with port 5555, exits.

- [ ] **Step 4: Write failing test for ClientLogger**

Create `cmd/multiserver/log_test.go`:

```go
func TestClientLoggerWritesFormattedLines(t *testing.T) {
    dir := t.TempDir()
    logger, err := NewClientLogger(dir, 1, "10.0.0.5", 54321)
    if err != nil {
        t.Fatal(err)
    }
    defer logger.Close()

    logger.Log("<", "hello world")
    logger.Log(">", "whoami")

    // read the file, verify two lines matching pattern:
    // [YYYY-MM-DD HH:MM:SS] < hello world
    // [YYYY-MM-DD HH:MM:SS] > whoami
}

func TestClientLoggerCreatesFileWithCorrectName(t *testing.T) {
    dir := t.TempDir()
    logger, err := NewClientLogger(dir, 3, "192.168.1.1", 9999)
    if err != nil {
        t.Fatal(err)
    }
    defer logger.Close()

    // verify file exists at: dir/client_3_192.168.1.1_9999.log
}

func TestClientLoggerHandlesMultilineText(t *testing.T) {
    dir := t.TempDir()
    logger, err := NewClientLogger(dir, 1, "10.0.0.1", 1234)
    if err != nil {
        t.Fatal(err)
    }
    defer logger.Close()

    logger.Log("<", "line1\nline2\nline3")
    // verify 3 separate log lines, each prefixed with timestamp and direction
}
```

- [ ] **Step 5: Run tests to verify they fail**

Run: `go test ./cmd/multiserver/ -run TestClientLogger -v`
Expected: FAIL — `NewClientLogger` undefined.

- [ ] **Step 6: Implement ClientLogger in `cmd/multiserver/log.go`**

`ClientLogger` struct: `mu sync.Mutex`, `file *os.File`. `NewClientLogger` creates the log directory if needed, opens `client_{cid}_{ip}_{port}.log` for append, writes connect line. `Log` splits on `\n`, writes each line with `[timestamp] direction text`. `Close` writes disconnect line and closes file. All methods lock `mu`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./cmd/multiserver/ -run TestClientLogger -v`
Expected: PASS

- [ ] **Step 8: Run race detector**

Run: `go test ./cmd/multiserver/ -run TestClientLogger -race -v`
Expected: PASS with no race warnings.

- [ ] **Step 9: Commit**

```bash
git add go.mod cmd/multiserver/main.go cmd/multiserver/log.go cmd/multiserver/log_test.go
git commit -m "feat: Go rewrite scaffold with flag parsing and ClientLogger"
```

---

### Task 2: Alias engine

**Files:**
- Create: `cmd/multiserver/aliases.go`
- Create: `cmd/multiserver/aliases_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type Alias struct { Windows, Linux string }`
  - `var defaultAliases map[string]*Alias` (all 23 aliases from Python version)
  - `func substituteArgs(cmd string, args []string) (string, error)` — replaces `{0}`, `{1}`, etc.
  - `func countFields(cmd string) int` — counts `{N}` placeholders

- [ ] **Step 1: Write failing tests for alias substitution**

Create `cmd/multiserver/aliases_test.go`:

```go
func TestSubstituteArgsSimple(t *testing.T) {
    result, err := substituteArgs("curl {0} -o {1}", []string{"http://example.com", "/tmp/file"})
    assert(t, err == nil)
    assert(t, result == "curl http://example.com -o /tmp/file")
}

func TestSubstituteArgsNoPlaceholders(t *testing.T) {
    result, err := substituteArgs("whoami /all", []string{})
    assert(t, err == nil)
    assert(t, result == "whoami /all")
}

func TestSubstituteArgsTooFewArgs(t *testing.T) {
    _, err := substituteArgs("curl {0} -o {1}", []string{"http://example.com"})
    assert(t, err != nil) // should report needs 2 args, got 1
}

func TestSubstituteArgsLiteralBraces(t *testing.T) {
    // a command with literal { that isn't a placeholder
    result, err := substituteArgs("echo hello", []string{})
    assert(t, err == nil)
    assert(t, result == "echo hello")
}

func TestCountFields(t *testing.T) {
    assert(t, countFields("curl {0} -o {1}") == 2)
    assert(t, countFields("whoami /all") == 0)
    assert(t, countFields("{0}") == 1)
    assert(t, countFields("{0} {0}") == 1) // same placeholder used twice = 1 unique field
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/multiserver/ -run "TestSubstitute|TestCountFields" -v`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement alias types and substitution in `cmd/multiserver/aliases.go`**

`substituteArgs`: scan for `{N}` with `regexp.MustCompile(`\{(\d+)\}`)`. Find max N, verify `len(args) > maxN`, then `ReplaceAllStringFunc` to swap each `{N}` with `args[N]`. `countFields`: same regex, return count of unique N values. Define `defaultAliases` map matching all 23 Python aliases exactly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/multiserver/ -run "TestSubstitute|TestCountFields" -v`
Expected: PASS

- [ ] **Step 5: Add Review Focus test — alias with `{0}` but zero args**

Add to test file:
```go
func TestSubstituteArgsZeroArgsWhenNeeded(t *testing.T) {
    _, err := substituteArgs("curl -fsSL {0} -o {1}", nil)
    assert(t, err != nil)
    // error message should mention "2" (needed) and "0" (got)
}
```

Run: `go test ./cmd/multiserver/ -run TestSubstituteArgsZeroArgs -v`
Expected: PASS (already handled by too-few-args logic).

- [ ] **Step 6: Commit**

```bash
git add cmd/multiserver/aliases.go cmd/multiserver/aliases_test.go
git commit -m "feat: alias engine with arg substitution and default alias table"
```

---

### Task 3: Server core — struct, accept loop, client lifecycle, shutdown

**Files:**
- Create: `cmd/multiserver/server.go`
- Create: `cmd/multiserver/client.go`
- Create: `cmd/multiserver/server_test.go`

**Interfaces:**
- Consumes:
  - `NewClientLogger(dir, cid, ip, port) (*ClientLogger, error)` from Task 1
  - `defaultAliases` from Task 2
- Produces:
  - `type Server struct` (as specified in design doc)
  - `type Client struct { ID int; Conn net.Conn; Addr net.Addr; OS string; Log *ClientLogger }`
  - `func NewServer(bind string, port, maxClients int, logDir string) *Server`
  - `func (s *Server) Start() error` — binds listener, starts accept loop
  - `func (s *Server) Shutdown()` — full graceful shutdown sequence
  - `func (s *Server) SendTo(cid int, msg string) error`
  - `func (s *Server) GetClient(cid int) *Client` — under RLock
  - `func (s *Server) ListClients() []*Client` — sorted snapshot under RLock
  - `func (s *Server) RemoveClient(cid int)` — under Lock, closes log
  - `func (s *Server) InteractCID() int32` — atomic load (-1 = none)
  - `func (s *Server) SetInteractCID(cid int32)` — atomic store

- [ ] **Step 1: Write failing test for Server lifecycle**

Create `cmd/multiserver/server_test.go`:

```go
func TestServerAcceptsConnection(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir()) // port 0 = OS picks
    err := s.Start()
    if err != nil {
        t.Fatal(err)
    }
    defer s.Shutdown()

    // dial the server
    conn, err := net.Dial("tcp", s.Addr().String())
    if err != nil {
        t.Fatal(err)
    }
    defer conn.Close()

    // wait briefly for accept loop to register client
    time.Sleep(100 * time.Millisecond)

    clients := s.ListClients()
    assert(t, len(clients) == 1)
    assert(t, clients[0].ID == 1)
}

func TestServerRejectsAtMaxClients(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 1, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn1, _ := net.Dial("tcp", s.Addr().String())
    defer conn1.Close()
    time.Sleep(100 * time.Millisecond)

    conn2, _ := net.Dial("tcp", s.Addr().String())
    defer conn2.Close()
    time.Sleep(100 * time.Millisecond)

    // only 1 client should be registered
    assert(t, len(s.ListClients()) == 1)
}

func TestServerSendTo(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn, _ := net.Dial("tcp", s.Addr().String())
    defer conn.Close()
    time.Sleep(100 * time.Millisecond)

    clients := s.ListClients()
    s.SendTo(clients[0].ID, "hello")

    buf := make([]byte, 64)
    conn.SetReadDeadline(time.Now().Add(time.Second))
    n, _ := conn.Read(buf)
    assert(t, string(buf[:n]) == "hello\n")
}

func TestServerGracefulShutdown(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()

    conn, _ := net.Dial("tcp", s.Addr().String())
    time.Sleep(100 * time.Millisecond)

    s.Shutdown()

    // conn should be closed by server
    buf := make([]byte, 1)
    _, err := conn.Read(buf)
    assert(t, err != nil) // EOF or connection reset
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/multiserver/ -run "TestServer" -v`
Expected: FAIL — `NewServer` undefined.

- [ ] **Step 3: Implement Client struct in `cmd/multiserver/client.go`**

`Client` struct with `ID`, `Conn`, `Addr`, `OS`, `Log` fields. `recvLoop(s *Server)` method: sets 1-second read deadline in a loop, reads into 4096-byte buffer, checks `s.running`, on data: if this client is interactive (`s.InteractCID() == int32(c.ID)`), write raw to `os.Stdout`; else format and print `[HH:MM:SS] [id] text`. On EOF or error: call `s.RemoveClient(c.ID)`. Replace invalid UTF-8 bytes using `strings.ToValidUTF8(string(buf[:n]), "�")`.

- [ ] **Step 4: Implement Server struct in `cmd/multiserver/server.go`**

`Server` struct as designed. `NewServer` initializes all fields, copies `defaultAliases`. `Start` creates log dir, binds listener, launches accept goroutine (adds to WaitGroup). Accept loop: accept conn, check max clients (reject if at limit), assign ID under Lock, create ClientLogger, create Client, store in map, launch `recvLoop` goroutine (adds to WaitGroup). `Shutdown`: set `running` to false, close listener, copy+close all conns under Lock, `WaitGroup.Wait` with 3s timeout via `time.AfterFunc`. `SendTo`: RLock to get conn, send outside lock, handle write error. `Addr() net.Addr` returns listener address (for tests with port 0).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/multiserver/ -run "TestServer" -v -timeout 30s`
Expected: PASS

- [ ] **Step 6: Run with race detector**

Run: `go test ./cmd/multiserver/ -run "TestServer" -race -v -timeout 30s`
Expected: PASS with no race warnings.

- [ ] **Step 7: Add Review Focus test — port already in use**

```go
func TestServerPortInUse(t *testing.T) {
    s1 := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s1.Start()
    defer s1.Shutdown()

    port := s1.Addr().(*net.TCPAddr).Port
    s2 := NewServer("127.0.0.1", port, 10, t.TempDir())
    err := s2.Start()
    assert(t, err != nil) // should fail with "address already in use"
}
```

Run: `go test ./cmd/multiserver/ -run TestServerPortInUse -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/multiserver/server.go cmd/multiserver/client.go cmd/multiserver/server_test.go
git commit -m "feat: Server core with accept loop, recv goroutine, send, and graceful shutdown"
```

---

### Task 4: Command handlers

**Files:**
- Create: `cmd/multiserver/commands.go`
- Create: `cmd/multiserver/commands_test.go`

**Interfaces:**
- Consumes:
  - `Server.ListClients()`, `Server.GetClient(cid)`, `Server.SendTo(cid, msg)`, `Server.RemoveClient(cid)`, `Server.InteractCID()`, `Server.SetInteractCID(cid)` from Task 3
  - `substituteArgs(cmd, args)`, `countFields(cmd)` from Task 2
- Produces:
  - `type CmdHandler func(s *Server, args []string) bool`
  - `var commands map[string]CmdHandler` — dispatch table
  - Individual handlers: `cmdList`, `cmdKick`, `cmdInteract`, `cmdOS`, `cmdAlias`, `cmdRun`, `cmdAll`, `cmdHelp`, `cmdExit`
  - `func (s *Server) Dispatch(line string) bool` — splits line, looks up handler, calls it; fallback `<id> <msg>`
  - `func ts() string` — `HH:MM:SS` timestamp helper

- [ ] **Step 1: Write failing tests for command dispatch**

Create `cmd/multiserver/commands_test.go`:

```go
func TestDispatchExit(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    assert(t, s.Dispatch("exit") == false)
    assert(t, s.Dispatch("quit") == false)
}

func TestDispatchHelp(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    assert(t, s.Dispatch("help") == true)
    assert(t, s.Dispatch("?") == true)
}

func TestDispatchUnknown(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    assert(t, s.Dispatch("garbage") == true) // prints usage, doesn't quit
}

func TestDispatchSendToClient(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn, _ := net.Dial("tcp", s.Addr().String())
    defer conn.Close()
    time.Sleep(100 * time.Millisecond)

    clients := s.ListClients()
    cid := clients[0].ID

    s.Dispatch(fmt.Sprintf("%d whoami", cid))

    buf := make([]byte, 64)
    conn.SetReadDeadline(time.Now().Add(time.Second))
    n, _ := conn.Read(buf)
    assert(t, string(buf[:n]) == "whoami\n")
}

func TestDispatchAll(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn1, _ := net.Dial("tcp", s.Addr().String())
    defer conn1.Close()
    conn2, _ := net.Dial("tcp", s.Addr().String())
    defer conn2.Close()
    time.Sleep(100 * time.Millisecond)

    s.Dispatch("all hello")

    for _, conn := range []net.Conn{conn1, conn2} {
        buf := make([]byte, 64)
        conn.SetReadDeadline(time.Now().Add(time.Second))
        n, _ := conn.Read(buf)
        assert(t, string(buf[:n]) == "hello\n")
    }
}

func TestDispatchOsSet(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn, _ := net.Dial("tcp", s.Addr().String())
    defer conn.Close()
    time.Sleep(100 * time.Millisecond)

    clients := s.ListClients()
    cid := clients[0].ID

    s.Dispatch(fmt.Sprintf("os %d linux", cid))
    client := s.GetClient(cid)
    assert(t, client.OS == "linux")
}

func TestDispatchRunAlias(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    s.Start()
    defer s.Shutdown()

    conn, _ := net.Dial("tcp", s.Addr().String())
    defer conn.Close()
    time.Sleep(100 * time.Millisecond)

    clients := s.ListClients()
    cid := clients[0].ID

    s.Dispatch(fmt.Sprintf("os %d linux", cid))
    s.Dispatch(fmt.Sprintf("run %d whoami", cid))

    buf := make([]byte, 64)
    conn.SetReadDeadline(time.Now().Add(time.Second))
    n, _ := conn.Read(buf)
    assert(t, string(buf[:n]) == "id\n") // "whoami" alias on linux = "id"
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/multiserver/ -run "TestDispatch" -v`
Expected: FAIL — `Dispatch` undefined.

- [ ] **Step 3: Implement command handlers in `cmd/multiserver/commands.go`**

Define `CmdHandler` type. Implement each handler:
- `cmdExit`: return false
- `cmdHelp`: print help text (match Python version's HELP string), return true
- `cmdList`: `s.ListClients()`, print formatted table with ID, IP:port, OS flag
- `cmdKick`: parse cid, `s.GetClient(cid)`, call `conn.Close()` if found
- `cmdInteract`: parse cid, verify client exists, `s.SetInteractCID(int32(cid))`
- `cmdOS`: no args = list all OS flags; with args = set OS for client (validate "windows"/"linux")
- `cmdAlias`: no args = list; "add" = parse `<name> <win> ::: <lin>`; "del" = remove
- `cmdRun`: parse cid + alias name + optional args, look up alias, get client OS, `substituteArgs`, `SendTo`
- `cmdAll`: broadcast `SendTo` to all clients

`Dispatch` method on Server: split line on whitespace, look up `commands[args[0]]`, call handler. Fallback: try `strconv.Atoi(args[0])` for `<id> <msg>` pattern.

All cid parsing uses `strconv.Atoi` with error check (no `isdigit`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/multiserver/ -run "TestDispatch" -v -timeout 30s`
Expected: PASS

- [ ] **Step 5: Add Review Focus test — `~.` at normal prompt**

```go
func TestDispatchTildeEscapeNormalMode(t *testing.T) {
    s := NewServer("127.0.0.1", 0, 10, t.TempDir())
    // not in interactive mode — ~. should be treated as unknown command
    assert(t, s.Dispatch("~.") == true) // doesn't quit, prints usage
}
```

Run: `go test ./cmd/multiserver/ -run TestDispatchTildeEscape -v`
Expected: PASS

- [ ] **Step 6: Run all tests with race detector**

Run: `go test ./cmd/multiserver/ -race -v -timeout 30s`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/multiserver/commands.go cmd/multiserver/commands_test.go
git commit -m "feat: command dispatch with all handlers — list, kick, os, alias, run, interact, all"
```

---

### Task 5: Console — readline, input loop, interactive mode

**Files:**
- Create: `cmd/multiserver/console.go`
- Modify: `cmd/multiserver/main.go` — wire everything together

**Interfaces:**
- Consumes:
  - `Server.Start()`, `Server.Shutdown()`, `Server.Dispatch(line)` from Tasks 3-4
  - `Server.InteractCID()`, `Server.SetInteractCID(cid)`, `Server.SendTo(cid, msg)` from Task 3
  - `github.com/chzyer/readline` — `readline.NewEx`, `readline.Instance`
- Produces:
  - `func (s *Server) RunConsole() error` — blocking readline loop, returns on exit/quit
  - `func (s *Server) DynamicCompleter() readline.AutoCompleter` — returns completer with commands + live client IDs + alias names
  - Fully working binary

- [ ] **Step 1: Add readline dependency**

Run:
```bash
cd cmd/multiserver
go get github.com/chzyer/readline
```

Verify: `go.sum` populated, `go.mod` has readline entry.

- [ ] **Step 2: Implement RunConsole in `cmd/multiserver/console.go`**

`RunConsole` method on `Server`:
- Create `readline.Instance` with config:
  - `Prompt`: dynamic — `> ` normally, `[N]$ ` in interactive mode
  - `AutoComplete`: `s.DynamicCompleter()`
  - `HistoryFile`: `~/.multiserver_history` (use `os.UserHomeDir()`)
  - `InterruptPrompt`: `"^C"`
  - `Stdin`: use readline's default (stdin)
- Main loop:
  - Set prompt based on `s.InteractCID()`
  - `rl.Readline()` — blocks for input
  - On `readline.ErrInterrupt`: if interactive, exit interactive mode (`SetInteractCID(-1)`), print message, continue; else ignore
  - On `io.EOF`: ignore (matches Python behavior — only exit/quit shuts down)
  - If interactive: check for `~.` → exit interactive; else `SendTo(interactCID, line)`
  - Else: `s.Dispatch(line)` — if false, break

`DynamicCompleter`: return `readline.NewPrefixCompleter` with static commands. For dynamic completion (client IDs, alias names), implement `readline.DynamicCompleteFunc` that reads live state under RLock.

- [ ] **Step 3: Wire main.go — parse flags, create Server, Start, RunConsole, Shutdown**

Update `cmd/multiserver/main.go`:
- Parse flags (already done in Task 1)
- `s := NewServer(bind, port, max, logdir)`
- Print banner: `[*] listening on {bind}:{port}`
- Handle OS signals: `SIGINT`, `SIGTERM` → call `s.Shutdown()` and exit (but only if not in interactive mode; in interactive mode, Ctrl-C is handled by readline)
- `s.Start()` — if error (port in use), print and exit
- `s.RunConsole()` — blocks
- `s.Shutdown()`

- [ ] **Step 4: Build and verify**

Run:
```bash
go build -o multiserver ./cmd/multiserver
go vet ./cmd/multiserver/
```

Expected: clean build, no vet warnings.

- [ ] **Step 5: Smoke test — manual verification**

Run: `./multiserver -p 5555`

In another terminal: `nc 127.0.0.1 5555`

Verify at console:
- `list` shows the client
- `1 whoami` sends to client (visible in nc)
- `os 1 linux` + `run 1 whoami` sends `id`
- `interact 1` enters interactive mode, `~.` exits
- `kick 1` disconnects
- `exit` shuts down cleanly

- [ ] **Step 6: Run full test suite with race detector**

Run: `go test ./cmd/multiserver/ -race -v -timeout 60s`
Expected: ALL PASS, zero races.

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add go.mod go.sum cmd/multiserver/console.go cmd/multiserver/main.go
git commit -m "feat: readline console with interactive mode, dynamic completion — fully working binary"
```

---

### Task 6: Polish, README, and cross-platform build verification

**Files:**
- Modify: `README.md` — updated for Go version
- Modify: `.gitignore` — add Go patterns
- Create: `Makefile` (optional, for cross-compilation convenience)

**Interfaces:**
- Consumes: fully working binary from Task 5
- Produces: updated docs, clean repo, verified builds

- [ ] **Step 1: Update .gitignore for Go**

Add: `multiserver`, `multiserver.exe`, `*.exe`, `dist/` to existing `.gitignore`.

- [ ] **Step 2: Update README.md**

Rewrite Setup section for Go:
```
go install github.com/RevanMardagh/multiserver/cmd/multiserver@latest
# or
go build -o multiserver ./cmd/multiserver
./multiserver -p 4444
```

Add Flags section documenting `-p`, `-b`, `-m`, `-l`. Keep command table identical. Keep alias table. Update Features list to include: single binary, cross-platform, race-free. Keep Security section.

- [ ] **Step 3: Cross-compile verification**

Run:
```bash
GOOS=linux GOARCH=amd64 go build -o dist/multiserver-linux-amd64 ./cmd/multiserver
GOOS=darwin GOARCH=arm64 go build -o dist/multiserver-darwin-arm64 ./cmd/multiserver
GOOS=windows GOARCH=amd64 go build -o dist/multiserver-windows-amd64.exe ./cmd/multiserver
```

Expected: three binaries in `dist/`, no errors.

- [ ] **Step 4: Final race check — all tests**

Run: `go test ./cmd/multiserver/ -race -v -count=3 -timeout 120s`
Expected: ALL PASS across 3 runs, zero races.

- [ ] **Step 5: Run go vet**

Run: `go vet ./cmd/multiserver/`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add README.md .gitignore
git commit -m "docs: update README for Go rewrite, add build instructions and flags"
```

- [ ] **Step 7: Clean up dist/ (don't commit binaries)**

```bash
rm -rf dist/
```
