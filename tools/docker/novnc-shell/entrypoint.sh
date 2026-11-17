#!/bin/bash
# Xvfb + openbox + passwordless x11vnc + websockify/noVNC on 0.0.0.0:6080; the shell restarts if it exits.
set -euo pipefail
unset XAUTHORITY
rm -f /tmp/.X1-lock /tmp/.X11-unix/X1
Xvfb :1 -screen 0 "${GEOMETRY:-1440x900}x24" -nolisten tcp &
for _ in $(seq 100); do [ -S /tmp/.X11-unix/X1 ] && break; sleep 0.1; done
openbox &
x11vnc -display :1 -nopw -forever -shared -rfbport 5900 -localhost -noxdamage -quiet &
websockify --web /usr/share/novnc 6080 localhost:5900 &
(while true; do /opt/crowbar/crowbar || true; sleep 1; done) &
wait -n
