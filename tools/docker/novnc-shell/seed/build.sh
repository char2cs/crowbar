#!/bin/bash
# Image build step: builds every seed profile into /opt/rig/profiles/<name>.tar (see README.md,
# "Seed profiles"). Each profile is the daemon home, the scripted agent's state and ~/fixtures.
#   empty     a migrated home with no projects; the scripted providers installed
#   baseline  empty + crowbar-seed (Crowbar Seed project) + crowbar-seed-chat (30-turn chat)
#   full      baseline + seed/full.py (everything the audit's Seed: cells ask for)
#   noproviders  baseline with every provider disabled
#   extras    baseline + seed/extras.py (a second project, README table, dotenv pair, rich chat, read-only ws)
set -euo pipefail
real=/opt/crowbar/crowbar-api.real
here=$(cd "$(dirname "$0")" && pwd)
profiles=/opt/rig/profiles
mkdir -p "$HOME/.rig" "$profiles"
sock=$("$real" paths | python3 -c 'import json, sys; print(json.load(sys.stdin)["socket"])')
api() { python3 /opt/rig/bin/rigapi.py "$@"; }
daemon=
start() {
  SCRIPTED_SPEED=0 "$real" serve --host "unix://$sock" >>"$HOME/.rig/seed-daemon.log" 2>&1 &
  daemon=$!
  for _ in $(seq 300); do python3 /opt/rig/bin/rigx.py health "$sock" >/dev/null 2>&1 && return; sleep 0.2; done
  echo "seed: the daemon did not become healthy" >&2
  exit 1
}
stop() { kill -TERM "$daemon"; wait "$daemon" || true; daemon=; }
trap '[[ -z $daemon ]] || kill -TERM "$daemon"' EXIT
save() {
  local parts=(.crowbar-rig) p
  for p in .scripted-agent fixtures; do [[ -e $HOME/$p ]] && parts+=("$p"); done
  # tar cannot read a mode-000 fixture: archive it readable and list it for `rig seed` to lock again.
  (cd "$HOME" && find "${parts[@]}" -type f -perm 000) >"$HOME/.rig-unreadable"
  (cd "$HOME" && xargs -r -d '\n' chmod 600 <"$HOME/.rig-unreadable")
  tar -S -cf "$profiles/$1.tar" -C "$HOME" "${parts[@]}" .rig-unreadable
  (cd "$HOME" && xargs -r -d '\n' chmod 000 <"$HOME/.rig-unreadable")
  echo "seed: profile $1 saved ($(numfmt --to=iec "$(stat -c %s "$profiles/$1.tar")"))"
}

chmod -R u+rwX "$CROWBAR_HOME" 2>/dev/null || true
rm -rf "$CROWBAR_HOME" "$HOME/.scripted-agent" "$HOME/fixtures"
python3 -c 'import sys; sys.path.insert(0, sys.argv[1]); import files; files.loose(sys.argv[2])' "$here" "$HOME/fixtures"

start
for d in /opt/rig/agent/*.yaml; do api POST /v0/settings/chat/descriptors "@$d" >/dev/null; done
stop
save empty

start
/opt/crowbar/crowbar-seed --host unix:// | tee "$HOME/.rig/seed.txt"
workspace=$(sed -n 's/^  workspace  .* (\(.*\))$/\1/p' "$HOME/.rig/seed.txt")
[[ -n $workspace ]] || { echo "seed: no workspace id in the seed report" >&2; exit 1; }
stop
/opt/crowbar/crowbar-seed-chat --workspace-id "$workspace" --turns 30 --tool-calls 4 | tee -a "$HOME/.rig/seed.txt"
save baseline

start
python3 "$here/full.py"
stop
save full

chmod -R u+rwX "$CROWBAR_HOME" "$HOME/fixtures" 2>/dev/null || true
rm -rf "$CROWBAR_HOME" "$HOME/.scripted-agent" "$HOME/fixtures"
tar -S -xf "$profiles/baseline.tar" -C "$HOME"
start
python3 - <<'PY'
import sys; sys.path.insert(0, "/opt/rig/bin")
import rigapi
api = rigapi.Api()
cur = api.get("/v0/settings/chat/providers")
api.put("/v0/settings/chat/providers", {"providers": [{"id": p["id"], "disabled": True, "mcpDisabled": not p["mcpEnabled"]} for p in cur]})
left = [p["id"] for p in api.get("/v0/settings/chat/providers") if p["enabled"]]
assert not left, f"providers still enabled: {left}"
print(f"seed: every provider disabled ({len(cur)})")
PY
stop
save noproviders

chmod -R u+rwX "$CROWBAR_HOME" "$HOME/fixtures" 2>/dev/null || true
rm -rf "$CROWBAR_HOME" "$HOME/.scripted-agent" "$HOME/fixtures"
tar -S -xf "$profiles/baseline.tar" -C "$HOME"
start
python3 "$here/extras.py"
stop
save extras

# The image keeps only the tars; the entrypoint and `rig seed` extract one.
rm -rf "$CROWBAR_HOME" "$HOME/.scripted-agent" "$HOME/fixtures" "$HOME/.rig-unreadable"
