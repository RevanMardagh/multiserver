# multiserver

Multi-client TCP listener for managing reverse shell connections. Go, single binary.

## Build & Test

```bash
go build ./cmd/multiserver           # build binary
go test ./cmd/multiserver/ -v        # run tests (39 tests)
go test ./cmd/multiserver/ -race -v  # race detector (needs CGO/C compiler)
go vet ./cmd/multiserver/            # static analysis
```

Cross-compile: `GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o multiserver-linux ./cmd/multiserver`

## Architecture

Single `main` package in `cmd/multiserver/`, 9 source files:

- `main.go` — entry point, CLI flags (`-p`, `-b`, `-m`, `-l`), signal handling
- `server.go` — `Server` struct, accept loop, `SendTo`, `RemoveClient`, `Shutdown`, reconnect tracking
- `client.go` — `Client` struct, `recvLoop` goroutine (1s read timeout), auto OS detection
- `console.go` — readline loop, interactive mode, dynamic tab completion
- `commands.go` — dispatch table (`map[string]CmdHandler`), all command handlers, `resolveClient` (ID or tag)
- `aliases.go` — `Alias` struct, 28 built-in aliases, `substituteArgs`, `countFields`
- `transfer.go` — `CaptureState` for base64 file download, `buildUploadCommands` for chunked upload
- `log.go` — `ClientLogger` with per-client `sync.Mutex`
- `features_test.go` — tests for OS detection, tag resolution, reconnect tracking

## Concurrency Model

- `Server.interactCID` — `atomic.Int32` (-1 = none). No lock needed.
- `Server.mu` — `sync.RWMutex` for client map. RLock for reads, Lock for writes.
- `Server.running` — `atomic.Bool`. Checked by recv loops and accept loop.
- Each `ClientLogger` has its own `sync.Mutex`. No global log lock.
- `sync.WaitGroup` tracks goroutines for graceful shutdown.

## Key Patterns

- Command dispatch: `var commands map[string]CmdHandler` in `commands.go`. Fallback parses `<id|tag> <message>`.
- Client resolution: `resolveClient()` tries numeric ID first, falls back to case-insensitive tag match.
- Alias arg substitution: `{0}`, `{1}` placeholders replaced via regex. `countFields` validates arg count.
- Interactive mode: atomic CID swap, raw bytes to stdout, `~.` or Ctrl-C to exit.
- Connection limits: accept then reject at max (don't leave conns in kernel backlog).
- Auto OS detection: first recv from client is checked for shell prompt signatures.
- Reconnect tracking: `Server.history` maps IP -> `ClientHistory` (tag, OS, groups). Restored on reconnect.
- File transfer: download uses base64 capture with unique markers; upload chunks base64 into shell commands.
- Client groups: `Server.groups` map, `gr`/`grun` for group-wide commands/aliases.

## Dependencies

One external: `github.com/chzyer/readline` (terminal readline with history, completion, concurrent-safe output).

## CI

- `.github/workflows/ci.yml` — vet + test + race on ubuntu + windows, every push/PR to master
- `.github/workflows/release.yml` — on `v*` tag: build 5 platform binaries, create GitHub Release
- All actions pinned to SHA. Write permission scoped to release job only.

## Release Process

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
# GitHub Actions builds binaries and creates release automatically
```

Every release MUST include a changelog in the GitHub Release notes covering new features, bug fixes, security changes, and breaking changes since the previous release.



## Security Notes

- Log files created 0600, log dir 0700
- Newlines escaped in log entries to prevent log injection
- History file created 0600, interactive mode input not saved to history
- No auth/TLS by design — intended for controlled environments only
