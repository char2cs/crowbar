#!/bin/bash
# Image build step: seed $CROWBAR_HOME (project, repo, two workspaces, review threads, and a long chat
# transcript in the feature workspace), then keep a copy for `rig reset`.
set -euo pipefail
real=/opt/crowbar/crowbar-api.real
mkdir -p "$HOME/.rig"
sock=$("$real" paths | python3 -c 'import json, sys; print(json.load(sys.stdin)["socket"])')
"$real" serve --host "unix://$sock" >"$HOME/.rig/seed-daemon.log" 2>&1 &
daemon=$!
for _ in $(seq 300); do python3 /opt/rig/bin/rigx.py health "$sock" >/dev/null 2>&1 && break; sleep 0.2; done
/opt/crowbar/crowbar-seed --host unix:// | tee "$HOME/.rig/seed.txt"
workspace=$(sed -n 's/^  workspace  .* (\(.*\))$/\1/p' "$HOME/.rig/seed.txt")
[[ -n $workspace ]] || { echo "seed: no workspace id in the seed report" >&2; exit 1; }
kill -TERM "$daemon"
wait "$daemon" || true
/opt/crowbar/crowbar-seed-chat --workspace-id "$workspace" --turns 30 --tool-calls 4 | tee -a "$HOME/.rig/seed.txt"
tar -cf /opt/rig/seeded-home.tar -C "$(dirname "$CROWBAR_HOME")" "$(basename "$CROWBAR_HOME")"
