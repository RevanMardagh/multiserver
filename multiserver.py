#!/usr/bin/env python3
"""
Tiny multi-client TCP listener with per-connection messaging and a
proper editable prompt (backspace, arrow keys, command history).

Setup:  pip install prompt_toolkit
Run:    python3 multiserver.py [port]        (default port 4444)

Console commands (type at the prompt):
    list                 show connected clients
    <id> <message>       send <message> to client <id>      e.g.  2 hello there
    all <message>        broadcast to every client
    kick <id>            disconnect client <id>
    exit                 shut everything down  (Ctrl-C / Ctrl-D ignored)

Incoming data from clients prints above the prompt as:  [<id>] <data>
"""

import os
import socket
import sys
import threading
from datetime import datetime

from prompt_toolkit import PromptSession
from prompt_toolkit.completion import WordCompleter
from prompt_toolkit.patch_stdout import patch_stdout


def ts():
    """Short HH:MM:SS timestamp for log lines."""
    return datetime.now().strftime("%H:%M:%S")

HOST = "0.0.0.0"
PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 4444
LOG_DIR = "logs"

clients = {}          # id -> socket
addrs = {}            # id -> (ip, port)
logfiles = {}         # id -> open file handle
next_id = 1
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
    try:
        while running.is_set():
            data = conn.recv(4096)
            if not data:
                break
            text = data.decode(errors="replace")
            log_event(cid, "<", text)
            if cid == interact_cid:
                # interactive mode: raw passthrough, no id/timestamp prefix
                sys.stdout.write(text)
                sys.stdout.flush()
            else:
                # patch_stdout() makes this appear cleanly above the prompt
                print(f"[{ts()}] [{cid}] {text.rstrip()}")
    except OSError:
        pass
    finally:
        global interact_cid
        if cid == interact_cid:
            interact_cid = None
            print(f"\n[*] interacted client {cid} gone, back to console")
        with lock:
            clients.pop(cid, None)
            addr = addrs.pop(cid, None)
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


def handle(line):
    """Process one console command. Return False to quit."""
    if line in ("quit", "exit"):
        return False

    parts0 = line.split()
    if parts0 and parts0[0] in ("interact", "i"):
        global interact_cid
        try:
            cid = int(parts0[1])
        except (IndexError, ValueError):
            print("[!] usage: interact <id>")
            return True
        with lock:
            conn = clients.get(cid)
        if conn is None:
            print(f"[!] no client with id {cid}")
            return True
        interact_cid = cid
        print(f"[*] interactive with client {cid}. '~.' or Ctrl-C to exit.")
        return True

    if line in ("list", "ls"):
        with lock:
            if not clients:
                print("    (no clients)")
            for cid in sorted(clients):
                ip, port = addrs[cid]
                print(f"    {cid}: {ip}:{port}")
        return True

    if line.startswith("all "):
        msg = line[4:]
        with lock:
            ids = list(clients)
        for cid in ids:
            send_to(cid, msg)
        return True

    if line.startswith("kick "):
        try:
            cid = int(line.split()[1])
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

    # default: "<id> <message>"
    parts = line.split(maxsplit=1)
    if len(parts) == 2 and parts[0].isdigit():
        send_to(int(parts[0]), parts[1])
    else:
        print("[!] usage: <id> <message>   (try 'list')")
    return True


def main():
    os.makedirs(LOG_DIR, exist_ok=True)
    running.set()

    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((HOST, PORT))
    srv.listen()
    print(f"[*] listening on {HOST}:{PORT}  (type 'list', '<id> msg', 'all msg', 'kick <id>', 'quit')")
    print(f"[*] logging per-client to ./{LOG_DIR}/")

    accept_thread = threading.Thread(target=accept_loop, args=(srv,), daemon=True)
    accept_thread.start()

    global interact_cid
    completer = WordCompleter(
        ["list", "ls", "all", "kick", "interact", "exit", "quit"],
        ignore_case=True,
    )
    session = PromptSession(completer=completer)
    # patch_stdout() routes all print() output (including from threads)
    # so it never corrupts the line you're editing.
    with patch_stdout():
        while True:
            prompt_str = f"[{interact_cid}]$ " if interact_cid is not None else "> "
            try:
                raw = session.prompt(prompt_str)
            except KeyboardInterrupt:
                if interact_cid is not None:
                    # Ctrl-C leaves interactive mode, not the server.
                    print(f"[*] left interactive mode (client {interact_cid})")
                    interact_cid = None
                continue
            except EOFError:
                # Ctrl-D: ignore. Only 'exit'/'quit' shuts down.
                continue

            if interact_cid is not None:
                # raw passthrough mode
                if raw.strip() == "~.":
                    print(f"[*] left interactive mode (client {interact_cid})")
                    interact_cid = None
                    continue
                send_to(interact_cid, raw)
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