#!/usr/bin/env bash
# Reference app only: restore the default shell view state (sidebar width/open, active project)
# and reload, so a scenario starts from the same pixels every time. Needs the MCP bridge.
# Usage: PROJECT_ID=<uuid> reset-reference.sh   (default: the first seeded project)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
export PARITY_CONTAINER=${PARITY_CONTAINER:-inventory-app} PARITY_OUT=${PARITY_OUT:-/tmp/parity-reset}
mkdir -p "$PARITY_OUT"; source "$here/lib.sh"
pid=${PROJECT_ID:-c2c5210c-d556-427f-ae34-866f9ea7d99f}
wipe=""
if [ "${RESET_LAYOUT:-0}" = 1 ]; then
  # tab/pane layout lives in IndexedDB; deleting it gives one chat view with an empty editor area
  wipe='indexedDB.databases().then(function(l){l.forEach(function(d){indexedDB.deleteDatabase(d.name)})});'
fi
js "$wipe"'localStorage.removeItem("sidebar-width"); localStorage.removeItem("sidebar-open");
    localStorage.setItem("crowbar.activeProject", JSON.stringify({state:{activeProjectId:"'"$pid"'"},version:0}));
    location.hash="#/ide/'"$pid"'/home"; setTimeout(function(){location.reload()},50); return "reloading"' >/dev/null
sleep 8
attach
