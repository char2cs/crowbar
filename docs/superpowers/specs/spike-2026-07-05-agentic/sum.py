#!/usr/bin/env python3
"""Summarize captured hook payloads into a legible id/source sequence."""
import json, re, sys

path = sys.argv[1]
try:
    txt = open(path).read()
except FileNotFoundError:
    print("(no hooks.log)"); sys.exit(0)

parts = re.split(r'===== EVENT=(\S+) ts=(\d+) pid=\d+ =====\n', txt)
seen = {}
order = []
print(f"{'#':>2}  {'EVENT':22} {'source':9} session_id")
print("-" * 70)
n = 0
for i in range(1, len(parts), 3):
    ev, ts, body = parts[i], parts[i+1], parts[i+2].strip()
    d = {}
    if body:
        try:
            d = json.loads(body.splitlines()[0])
        except Exception:
            pass
    sid = d.get("session_id", "-")
    src = d.get("source", "-")
    n += 1
    tag = ""
    if sid not in seen and sid != "-":
        seen[sid] = n
        if len(seen) > 1:
            tag = "  <-- NEW id (move detected)"
    print(f"{n:>2}  {ev:22} {src:9} {sid}{tag}")
print("-" * 70)
print(f"distinct session_ids seen: {len([k for k in seen if k!='-'])}")
