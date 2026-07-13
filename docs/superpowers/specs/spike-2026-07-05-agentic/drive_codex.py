#!/usr/bin/env python3
"""Codex Phase-0 spike: relocated CODEX_HOME injection, real PTY, no headless."""
import os, sys, time, pty, select, fcntl, termios, struct, signal, subprocess

SPIKE = "/private/tmp/claude-501/-Users-char2cs--crowbar-projects-71244879-4ed1-416e-a6b4-60eeac355663-9e6b3e9c-0f25-47f4-989f-45c922542272-workspaces-43e4091a-26c2-4c70-ad4a-833e690443a0-worktree/ddef59a4-f47f-4fdc-ad7c-b2e61a68c3af/scratchpad/spike"
CODEX = "/Users/char2cs/.local/bin/codex"

# (seconds_to_pump_before, bytes_to_send_after or None)
SCRIPT = [
    (18.0, b"respond with just: ok"),   # boot, then type turn 1
    (2.5,  b"\r"),                       # submit -> SessionStart(id0)
    (14.0, b"/new"),                     # MOVE: start a new conversation within this segment
    (2.0,  b"\r"),                       # execute /new
    (5.0,  b"respond with just: ok"),    # turn 2 (forces the new session if /new is lazy)
    (2.5,  b"\r"),                       # submit -> SessionStart(id1)  <-- should differ from id0
    (45.0, None),
]

def main():
    raw_path = os.path.join(SPIKE, "raw_codex.log")
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    env = dict(os.environ)
    env["TERM"] = "xterm-256color"
    env["CODEX_HOME"] = os.path.join(SPIKE, "codex_home")
    args = [CODEX, "--dangerously-bypass-hook-trust"]
    p = subprocess.Popen(args, stdin=slave, stdout=slave, stderr=slave,
                         env=env, cwd=SPIKE, close_fds=True, preexec_fn=os.setsid)
    os.close(slave)
    raw = open(raw_path, "wb")

    def pump(until):
        while time.time() < until:
            r, _, _ = select.select([master], [], [], 0.2)
            if master in r:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    return False
                if not data:
                    return False
                raw.write(data); raw.flush()
        return True

    for delay, payload in SCRIPT:
        pump(time.time() + delay)
        if payload is not None:
            os.write(master, payload)
    pump(time.time() + 1.5)

    for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(os.getpgid(p.pid), sig)
        except Exception:
            break
        time.sleep(0.8)
        if p.poll() is not None:
            break
    raw.close()
    print("codex driver done")

if __name__ == "__main__":
    main()
