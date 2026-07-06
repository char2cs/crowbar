#!/usr/bin/env python3
"""Does Claude persist when it mints its OWN session id (no --session-id)?
If yes: record-not-assign fixes persistence AND enables --resume."""
import os, time, pty, select, fcntl, termios, struct, signal, subprocess, json

SPIKE = "/private/tmp/claude-501/-Users-char2cs--crowbar-projects-71244879-4ed1-416e-a6b4-60eeac355663-9e6b3e9c-0f25-47f4-989f-45c922542272-workspaces-43e4091a-26c2-4c70-ad4a-833e690443a0-worktree/ddef59a4-f47f-4fdc-ad7c-b2e61a68c3af/scratchpad/spike"
CLAUDE = "/Users/char2cs/.local/bin/claude"
PROJ = os.path.expanduser("~/.claude/projects")
HOOKS_LOG = os.path.join(SPIKE, "hooks.log")
PROMPT = "Remember this codeword: FALCON-7719. Reply with only: acknowledged."

def count_stop():
    try: return open(HOOKS_LOG).read().count("EVENT=Stop ")
    except FileNotFoundError: return 0

def last_session_id():
    sid = None
    try: lines = open(HOOKS_LOG)
    except FileNotFoundError: return None
    for ln in lines:
        ln = ln.strip()
        if ln.startswith("{"):
            try:
                d = json.loads(ln)
                if d.get("hook_event_name") == "SessionStart":
                    sid = d.get("session_id")
            except Exception: pass
    return sid

def find_id(sid):
    if not sid: return []
    key = sid[:13]
    out = []
    for root, _, files in os.walk(PROJ):
        for f in files:
            if f.endswith(".jsonl") and key in f:
                p = os.path.join(root, f)
                try: out.append((os.stat(p).st_size, p))
                except OSError: pass
    return out

def main():
    open(HOOKS_LOG, "w").close()
    env = dict(os.environ); env["TERM"] = "xterm-256color"; env["PWD"] = SPIKE
    # Strip the nested-Claude-Code markers we inherited (the spike runs inside Claude
    # Code; Crowbar's Go daemon would spawn with a clean env). CLAUDE_CODE_CHILD_SESSION
    # makes the child treat itself as ephemeral and skip transcript persistence.
    for k in [k for k in env if k.startswith("CLAUDE") or k == "CLAUDECODE"]:
        env.pop(k, None)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 45, 140, 0, 0))
    # NOTE: no --session-id -> Claude assigns its own, native, persisted id
    p = subprocess.Popen([CLAUDE, "--settings", os.path.join(SPIKE, "settings.json")],
                         stdin=slave, stdout=slave, stderr=slave, env=env, cwd=SPIKE,
                         close_fds=True, preexec_fn=os.setsid)
    os.close(slave)
    sends = [(5.0, b"\r"), (9.0, PROMPT.encode()), (11.0, b"\r")]
    t0 = time.time(); pending = list(sends); base = count_stop()
    def pump(dur):
        end = time.time() + dur
        while time.time() < end:
            now = time.time()
            for it in list(pending):
                if now - t0 >= it[0]:
                    pending.remove(it)
                    try: os.write(master, it[1])
                    except OSError: pass
            r, _, _ = select.select([master], [], [], 0.2)
            if master in r:
                try:
                    if not os.read(master, 65536): return
                except OSError: return
    while count_stop() <= base and time.time() - t0 < 120:
        pump(0.5)
    sid = last_session_id()
    print("turn complete:", count_stop() > base, "| native session_id:", sid)
    print("WHILE ALIVE, native session file:", find_id(sid) or "(none yet)")
    pump(3.0)
    print("after 3s alive:", find_id(sid) or "(none)")
    # clean exit
    for b in (b"\x04", b"\x04"):
        try: os.write(master, b)
        except OSError: break
        pump(1.0)
    gt = time.time() + 8
    while time.time() < gt and p.poll() is None:
        pump(0.5)
    time.sleep(1.0)
    found = find_id(sid)
    print("AFTER CLEAN EXIT native session file:", found or "(none)")
    if found:
        txt = open(found[0][1]).read()
        print("  FALCON in persisted transcript:", "FALCON-7719" in txt, "| bytes:", len(txt))
    for sg in (signal.SIGTERM, signal.SIGKILL):
        if p.poll() is not None: break
        try: os.killpg(os.getpgid(p.pid), sg)
        except Exception: break
        time.sleep(0.6)

if __name__ == "__main__":
    main()
