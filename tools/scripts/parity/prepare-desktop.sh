#!/usr/bin/env bash
# One-time desktop setup inside the container: a single workspace and no XFCE grabs of the
# chords the app uses (Ctrl+Alt+Arrow is pane navigation), so scripted keys reach the app.
set -euo pipefail
c=${PARITY_CONTAINER:?set PARITY_CONTAINER}
docker exec -e DISPLAY=${PARITY_DISPLAY:-:1} "$c" bash -c '
  xfconf-query -c xfwm4 -p /general/workspace_count -s 1
  for k in Left Right Up Down; do
    xfconf-query -c xfce4-keyboard-shortcuts -p "/xfwm4/custom/<Primary><Alt>$k" -r 2>/dev/null || true
  done
  xdotool set_desktop 0'
