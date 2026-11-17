# multiserver — Feature Roadmap

## Priority (implementing now)

- [ ] **Auto OS detection** — sniff first response for shell prompt signatures (`C:\`, `PS `, `$`, `#`) to auto-set OS flag
- [ ] **Client tagging** — `tag <id> <name>` command, reference clients by name in all commands
- [ ] **File download/upload** — base64-chunked file transfer over existing TCP connection
- [ ] **Client groups** — `group add <name> <ids...>`, run commands/aliases on groups
- [ ] **Reconnect tracking** — match reconnecting clients by IP, preserve tags/OS/history

## Backlog

- [ ] **Config file persistence** — YAML config to persist aliases, groups, tags across restarts
- [ ] **Session recording/replay** — `replay <id>` with timing data from logs
- [ ] **SOCKS proxy** — tunnel traffic through a client connection for pivoting
