#!/usr/bin/env python3
"""Crowbar Phase-0: end-to-end provider-switch spike.
Claude(A) --seed FALCON--> [switch: opaque transcript handoff] --> Codex --seed OTTER-->
  [switch-back: --resume A + --append-system-prompt <codex delta + sentinel>] --> Claude.
Proves: cross-provider opaque read, forward + backward handoff injection lands,
native resume (Case 1), and whether appended context pollutes the session file.
Never headless; never types the big blobs (they ride as argv). Turns sync on Stop hooks."""
import os, sys, time, pty, select, fcntl, termios, struct, signal, subprocess, json, re

SPIKE = "/private/tmp/claude-501/-Users-char2cs--crowbar-projects-71244879-4ed1-416e-a6b4-60eeac355663-9e6b3e9c-0f25-47f4-989f-45c922542272-workspaces-43e4091a-26c2-4c70-ad4a-833e690443a0-worktree/ddef59a4-f47f-4fdc-ad7c-b2e61a68c3af/scratchpad/spike"
CLAUDE = "/Users/char2cs/.local/bin/claude"
CODEX = "/Users/char2cs/.local/bin/codex"
HOOKS_LOG = os.path.join(SPIKE, "hooks.log")
SETTINGS = os.path.join(SPIKE, "settings.json")
CODEX_HOME = os.path.join(SPIKE, "codex_home")
SID_A = "aaaaaaaa-1111-4aaa-8aaa-aaaaaaaaaaaa"

FALCON = "FALCON-7719"       # native to Claude session A
OTTER = "OTTER-3304"         # born on Codex; only reaches Claude via the appended delta
SENTINEL = "CRWB-DELTA-SENTINEL-4821"  # embedded in the appended system prompt -> pollution probe

PROMPT1 = ("Please remember this exact codeword for the rest of our conversation: "
           + FALCON + ". Reply with only the word: acknowledged.")
PROMPT2_SUFFIX = ("\n\n=== END OF HANDED-OFF TRANSCRIPT ===\nThe text above this line is the "
                  "complete transcript of a conversation handed off to you from another AI "
                  "assistant. Read it carefully, then do two things: (1) State the exact codeword "
                  "that was mentioned in that conversation. (2) Remember this new codeword for "
                  "later: " + OTTER + ". Keep your reply to one or two sentences.")
PROMPT3 = ("List every distinct codeword that has appeared anywhere in this entire conversation, "
           "including any context that was appended or handed to you. Reply with just the "
           "codewords, comma-separated.")


def parse_hooks():
    try:
        txt = open(HOOKS_LOG).read()
    except FileNotFoundError:
        return []
    parts = re.split(r'===== EVENT=(\S+) ts=(\d+) pid=\d+ =====\n', txt)
    out = []
    for i in range(1, len(parts), 3):
        ev, ts, body = parts[i], parts[i + 1], parts[i + 2].strip()
        d = {}
        if body:
            try:
                d = json.loads(body.splitlines()[0])
            except Exception:
                pass
        out.append((ev, int(ts), d))
    return out


def count_event(label):
    # exact label match — "Stop" must NOT match "CODEX_Stop"
    return sum(1 for ev, _, _ in parse_hooks() if ev == label)


def last_payload(label):
    hit = None
    for ev, _, d in parse_hooks():
        if ev == label:
            hit = d
    return hit or {}


def set_winsize(fd, rows=45, cols=140):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def run_segment(name, argv, env, stop_label, sends, timeout=130, graceful=True):
    """sends = list of (t_offset_seconds, bytes). Segment ends when a NEW stop_label
    hook appears (turn complete) or timeout. graceful=True quits the TUI cleanly
    (double Ctrl-C) so it can flush its transcript before reaping. Returns True if a
    stop was seen."""
    raw_path = os.path.join(SPIKE, "raw_%s.log" % name)
    base = count_event(stop_label)
    master, slave = pty.openpty()
    set_winsize(slave)
    p = subprocess.Popen(argv, stdin=slave, stdout=slave, stderr=slave,
                         env=env, cwd=SPIKE, close_fds=True, preexec_fn=os.setsid)
    os.close(slave)
    raw = open(raw_path, "wb")
    t0 = time.time()
    done_at = None
    pending = list(sends)
    stopped = False
    while True:
        now = time.time()
        # fire scheduled inputs (text and Enter must be SEPARATE writes: a trailing
        # \r in the same write as pasted text is a literal newline, not a submit)
        for item in list(pending):
            if now - t0 >= item[0]:
                pending.remove(item)
                try:
                    os.write(master, item[1])
                except OSError:
                    pending.clear()
                    break
        # drain child output
        r, _, _ = select.select([master], [], [], 0.2)
        if master in r:
            try:
                data = os.read(master, 65536)
                if data:
                    raw.write(data); raw.flush()
            except OSError:
                break
        # child exited on its own (e.g. --resume of an empty session errors out)
        if p.poll() is not None and not stopped:
            time.sleep(0.4)
            break
        # completion: a fresh stop hook fired
        if not stopped and count_event(stop_label) > base:
            stopped = True
            done_at = time.time() + 2.5   # grace for transcript + payload flush
        if stopped and time.time() >= done_at:
            break
        if now - t0 > timeout:
            break
    if stopped and graceful:
        # Claude persists its transcript only on a clean exit. Quit the TUI (double
        # Ctrl-C) and let it flush before we reap; Codex writes incrementally so it
        # tolerates an abrupt kill.
        for _ in range(2):
            try:
                os.write(master, b"\x03")
            except OSError:
                break
            time.sleep(0.5)
        gt = time.time() + 9
        while time.time() < gt and p.poll() is None:
            r, _, _ = select.select([master], [], [], 0.2)
            if master in r:
                try:
                    os.read(master, 65536)
                except OSError:
                    break
    for sg in (signal.SIGINT, signal.SIGTERM, signal.SIGKILL):
        if p.poll() is not None:
            break
        try:
            os.killpg(os.getpgid(p.pid), sg)
        except Exception:
            break
        time.sleep(0.6)
    raw.close()
    return stopped


def claude_answer_from_transcript(path):
    """Fallback: last assistant text block from a Claude jsonl transcript."""
    try:
        lines = open(path).read().splitlines()
    except Exception:
        return ""
    ans = ""
    for ln in lines:
        try:
            o = json.loads(ln)
        except Exception:
            continue
        if o.get("type") == "assistant":
            msg = o.get("message", {})
            for blk in msg.get("content", []):
                if isinstance(blk, dict) and blk.get("type") == "text":
                    ans = blk.get("text", "")
    return ans


def main():
    open(HOOKS_LOG, "w").close()  # fresh
    base_env = dict(os.environ); base_env["TERM"] = "xterm-256color"
    # Strip inherited nested-Claude-Code markers (spike runs inside Claude Code; the
    # child-session flag suppresses transcript persistence). Crowbar's Go daemon is clean.
    for k in [k for k in base_env if k.startswith("CLAUDE") or k == "CLAUDECODE"]:
        base_env.pop(k, None)

    # ---------- SEG 1: Claude(A) seeds FALCON ----------
    print(">> SEG1 Claude(A): seeding", FALCON)
    env1 = dict(base_env)
    run_segment("s1_claude", [CLAUDE, "--settings", SETTINGS],  # native id (record-not-assign)
                env1, "Stop",
                [(5.0, b"\r"), (9.0, PROMPT1.encode()), (11.0, b"\r")])
    ss_a = last_payload("SessionStart")
    a_id = ss_a.get("session_id", "")
    transcript_a = ss_a.get("transcript_path", "")
    stop1 = last_payload("Stop")
    print("   A id:", ss_a.get("session_id"), "| transcript:", transcript_a)
    print("   Stop.last_assistant_message:", repr(stop1.get("last_assistant_message"))[:120])
    try:
        ta_txt = open(transcript_a).read()
    except Exception:
        ta_txt = ""
    print("   transcript_A bytes:", len(ta_txt), "| FALCON in transcript:", FALCON in ta_txt)

    # ---------- SEG 2: Codex reads opaque handoff, seeds OTTER ----------
    print(">> SEG2 Codex: opaque handoff (", len(ta_txt), "bytes ), asking for the codeword")
    handoff = ta_txt if ta_txt else "(prior transcript unavailable)"
    codex_prompt = handoff + PROMPT2_SUFFIX
    env2 = dict(base_env); env2["CODEX_HOME"] = CODEX_HOME
    run_segment("s2_codex",
                [CODEX, "--dangerously-bypass-hook-trust", codex_prompt],
                env2, "CODEX_Stop",
                [(19.0, b"\r")], timeout=150, graceful=False)
    codex_stop = last_payload("CODEX_Stop")
    codex_ss = last_payload("CODEX_SessionStart")
    codex_answer = codex_stop.get("last_assistant_message", "") or ""
    codex_transcript = codex_ss.get("transcript_path", "") or codex_stop.get("transcript_path", "")
    print("   codex id:", codex_ss.get("session_id"))
    print("   codex answer:", repr(codex_answer)[:300])
    try:
        codex_txt = open(codex_transcript).read()
    except Exception:
        codex_txt = ""
    print("   codex transcript bytes:", len(codex_txt))

    # ---------- SEG 3: switch back — resume A + append codex delta ----------
    print(">> SEG3 Claude(--resume A + --append-system-prompt delta)")
    delta = ("[BEGIN HANDED-BACK CONTEXT | marker=" + SENTINEL + "]\n"
             "While you were paused, the conversation continued on a different AI assistant. "
             "Below is that assistant's full session transcript:\n" + (codex_txt or codex_answer) +
             "\n[END HANDED-BACK CONTEXT]")
    env3 = dict(base_env)
    run_segment("s3_claude",
                [CLAUDE, "--settings", SETTINGS, "--resume", a_id,
                 "--append-system-prompt", delta],
                env3, "Stop",
                [(5.0, b"\r"), (10.0, PROMPT3.encode()), (12.0, b"\r")], timeout=150)
    resume_ss = last_payload("SessionStart")
    stop3 = last_payload("Stop")
    claude_answer = stop3.get("last_assistant_message") or claude_answer_from_transcript(transcript_a)
    print("   resume SessionStart source:", resume_ss.get("source"), "| id:", resume_ss.get("session_id"))
    print("   claude final answer:", repr(claude_answer)[:300])
    try:
        ta_after = open(transcript_a).read()
    except Exception:
        ta_after = ""

    # ---------- SCORECARD ----------
    fwd_ok = FALCON in codex_answer
    resume_ok = (resume_ss.get("source") == "resume" and resume_ss.get("session_id") == a_id)
    delta_ok = OTTER in (claude_answer or "")
    continuity_ok = FALCON in (claude_answer or "")
    pollutes = SENTINEL in ta_after
    print("\n================ SCORECARD ================")
    print(f"[{'PASS' if fwd_ok else 'FAIL'}] Forward handoff + cross-provider opaque read  (Codex echoed {FALCON}): {fwd_ok}")
    print(f"[{'PASS' if resume_ok else 'FAIL'}] Native resume / Case-1 (SessionStart source=resume, id=A): {resume_ok}")
    print(f"[{'PASS' if delta_ok else 'FAIL'}] Backward delta injection landed  (Claude knew {OTTER}, only from delta): {delta_ok}")
    print(f"[{'PASS' if continuity_ok else 'FAIL'}] Continuity  (Claude still knew {FALCON} after round trip): {continuity_ok}")
    print(f"[POLLUTION] appended system prompt persisted into session file: {pollutes}  "
          f"({'POLLUTES' if pollutes else 'clean / per-invocation'})")
    print("===========================================")


if __name__ == "__main__":
    main()
