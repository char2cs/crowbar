#!/bin/bash
# Xvfb wide enough for both app windows side by side, openbox, passwordless x11vnc, noVNC on 6080,
# the one shared daemon, then both apps at the rig's standard size.
set -uo pipefail
unset XAUTHORITY
W=${RIG_W:-1280} H=${RIG_H:-800} GAP=${RIG_GAP:-20}
mkdir -p "$HOME/.rig"
[[ -d $CROWBAR_HOME ]] || rig _extract-profile "${RIG_PROFILE:-baseline}"
rm -f /tmp/.X1-lock /tmp/.X11-unix/X1
Xvfb :1 -screen 0 "$((2 * W + GAP + 20))x$((H + 100))x24" -nolisten tcp &
essential=($!)
for _ in $(seq 100); do [[ -S /tmp/.X11-unix/X1 ]] && break; sleep 0.1; done
openbox --config-file /opt/rig/rc.xml &
essential+=($!)
x11vnc -display :1 -nopw -forever -shared -rfbport 5900 -localhost -noxdamage -quiet &
essential+=($!)
websockify --web /usr/share/novnc 6080 localhost:5900 &
essential+=($!)
rig _daemon-loop &
essential+=($!)
(
  rig http start
  rig _wait-daemon || exit 1
  if [[ -e $HOME/.rig/initialised ]]; then rig start all; else rig init; fi
) >>"$HOME/.rig/entrypoint.log" 2>&1 &
# The container lives as long as the desktop and the daemon loop do (the apps come and go).
wait -n "${essential[@]}"
