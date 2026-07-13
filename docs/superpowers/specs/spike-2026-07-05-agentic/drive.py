#!/usr/bin/env python3
"""Crowbar Phase-0 spike driver: spawn a vendor CLI in a real PTY, drive a
scripted sequence, capture raw TUI output. Hook payloads land in hooks.log via
capture.sh. Never uses headless flags."""
import os, sys, time, pty, select, fcntl, termios, struct, signal, subprocess

SPIKE = "/private/tmp/claude-501/-Users-char2cs--crowbar-projects-71244879-4ed1-416e-a6b4-60eeac355663-9e6b3e9c-0f25-47f4-989f-45c922542272-workspaces-43e4091a-26c2-4c70-ad4a-833e690443a0-worktree/ddef59a4-f47f-4fdc-ad7c-b2e61a68c3af/scratchpad/spike"
CLAUDE = "/Users/char2cs/.local/bin/claude"

# Scripted actions: (seconds_to_pump_before, bytes_to_send_after or None)
# Overridden per-run by editing this list.
SCRIPT = [
    (4.0, b"\r"),         # accept the "trust this folder?" prompt
    (8.0, b"/clear\r"),   # MOVE 1 -> new id
    (5.0, b"/clear\r"),   # MOVE 2 -> another new id (shows repeatable, distinct ids)
    (5.0, None),
]

def main():
    sid = sys.argv[1] if len(sys.argv) > 1 else "11111111-1111-4111-8111-111111111111"
    raw_path = os.path.join(SPIKE, "raw.log")
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    env = dict(os.environ); env["TERM"] = "xterm-256color"
    args = [CLAUDE, "--settings", os.path.join(SPIKE, "settings.json"),
            "--session-id", sid]
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
    print("driver done; session-id=%s" % sid)

if __name__ == "__main__":
    main()
