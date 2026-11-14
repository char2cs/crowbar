#!/usr/bin/env bash
# Captures REAL codex app-server traffic into fixture files, by driving an actual turn.
#
# Run manually; the output is committed. NEVER hand-write a file in the fixtures
# directory. Synthetic fixtures have hidden real output shape in this repo before, and
# the entire value of the replay test is that these came off the wire.
#
# Requires `codex login`. Costs one trivial model turn.
set -euo pipefail
OUT="${1:?usage: capture-codex-fixtures.sh <outdir>}"
command -v codex >/dev/null || { echo "codex not on PATH" >&2; exit 1; }
codex --version

python3 - "$OUT" <<'PY'
import json, os, pathlib, subprocess, sys, threading, time

out = pathlib.Path(sys.argv[1]); out.mkdir(parents=True, exist_ok=True)
work = pathlib.Path("/tmp/codex-fixture-cwd"); work.mkdir(exist_ok=True)

p = subprocess.Popen(["codex", "app-server", "--listen", "stdio://"],
                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
def send(o):
    p.stdin.write(json.dumps(o) + "\n"); p.stdin.flush()

frames, lock = [], threading.Lock()
def reader():
    for line in p.stdout:
        line = line.strip()
        if not line: continue
        try: f = json.loads(line)
        except ValueError: continue
        with lock: frames.append(f)
threading.Thread(target=reader, daemon=True).start()

send({"jsonrpc":"2.0","id":1,"method":"initialize",
      "params":{"clientInfo":{"name":"crowbar-fixtures","title":"fixtures","version":"0.0.1"}}})
send({"jsonrpc":"2.0","method":"initialized","params":{}})
# sandbox is a STRING variant, and input is a SEQUENCE — both learned the hard way
# from the server's own -32600 replies.
send({"jsonrpc":"2.0","id":2,"method":"thread/start",
      "params":{"cwd":str(work),"approvalPolicy":"never","sandbox":"read-only"}})

tid = None
for _ in range(120):
    with lock: fs = list(frames)
    for f in fs:
        if f.get("method") == "thread/started":
            tid = f["params"]["thread"]["id"]
    if tid: break
    time.sleep(0.25)
if not tid:
    print("no thread/started; aborting", file=sys.stderr); p.kill(); sys.exit(1)

send({"jsonrpc":"2.0","id":3,"method":"turn/start",
      "params":{"threadId":tid,"input":[{"type":"text","text":"Reply with exactly: OK"}]}})
for _ in range(300):
    with lock: fs = list(frames)
    if any(f.get("method") == "turn/completed" for f in fs): break
    time.sleep(0.5)
time.sleep(1); p.kill()

home = os.path.expanduser("~")
def redact(o):
    # Machine-specific absolute paths ONLY. Shape is never touched.
    if isinstance(o, dict):  return {k: redact(v) for k, v in o.items()}
    if isinstance(o, list):  return [redact(v) for v in o]
    if isinstance(o, str):   return o.replace(home, "/REDACTED-HOME")
    return o

with lock: fs = list(frames)
seen = {}
for f in fs:
    if "method" not in f:
        continue  # request/response pairs are not inbound events
    seen.setdefault(f["method"].replace("/", "_"), f)

for old in out.glob("*.json"): old.unlink()
for name, frame in sorted(seen.items()):
    (out / f"{name}.json").write_text(json.dumps(redact(frame), indent=2) + "\n")
print(f"captured {len(seen)} frames into {out}")
for n in sorted(seen): print("  ", n)
PY
