#!/usr/bin/env bash
# Shared helpers for parity scenarios. Runs on the HOST; every X11 call is a
# `docker exec` into the desktop container. Sourced by run-scenario.sh.
#
# Env:
#   PARITY_CONTAINER   container with Xvnc desktop + xdotool + imagemagick (+ ffmpeg)
#   PARITY_DISPLAY     X display inside the container (default :1)
#   PARITY_WINDOW      window-name substring of the app under test
#   PARITY_W/PARITY_H  client size to normalise to (default 1200x800)
#   PARITY_OUT         output directory for shots/frames/index.tsv
set -euo pipefail

PARITY_CONTAINER=${PARITY_CONTAINER:?set PARITY_CONTAINER}
PARITY_DISPLAY=${PARITY_DISPLAY:-:1}
PARITY_WINDOW=${PARITY_WINDOW:-Crowbar}
PARITY_W=${PARITY_W:-1200}
PARITY_H=${PARITY_H:-800}
PARITY_OUT=${PARITY_OUT:?set PARITY_OUT}
PARITY_SETTLE=${PARITY_SETTLE:-0.6}
mkdir -p "$PARITY_OUT"

x() { docker exec -e DISPLAY="$PARITY_DISPLAY" "$PARITY_CONTAINER" "$@"; }

# Pick the app window: the visible window whose name contains PARITY_WINDOW and
# whose geometry is the largest (WebKit also exposes a tiny child window).
win_id() {
  x bash -c '
    best=""; area=0
    for id in $(xdotool search --onlyvisible --name "'"$PARITY_WINDOW"'"); do
      eval "$(xdotool getwindowgeometry --shell $id)"
      a=$((WIDTH*HEIGHT)); [ $a -gt $area ] && { area=$a; best=$id; }
    done
    echo $best'
}

WID=""
attach() {
  WID=$(win_id)
  [ -n "$WID" ] || { echo "parity: window '$PARITY_WINDOW' not found" >&2; exit 1; }
  x xdotool windowfocus "$WID" || true
  x xdotool windowmove "$WID" 5 40 || true
  x xdotool windowsize "$WID" "$PARITY_W" "$PARITY_H"
  # a freshly mapped WebKit window paints black until its size changes once
  x xdotool windowsize "$WID" $((PARITY_W+1)) "$PARITY_H"; sleep 0.3
  x xdotool windowsize "$WID" "$PARITY_W" "$PARITY_H"; sleep 1
}

settle() { sleep "${1:-$PARITY_SETTLE}"; }

# Window-relative input. Coordinates are client pixels, origin top-left.
move()   { x xdotool mousemove --window "$WID" "$1" "$2"; }
# down and up are separated: the webview ignores a zero-length press
click()  { move "$1" "$2"; settle 0.25; x xdotool mousedown "${3:-1}"; sleep 0.08; x xdotool mouseup "${3:-1}"; }
scroll() { move "$1" "$2"; x xdotool click --repeat "${4:-5}" --delay 40 "$3"; }  # $3: 4 up, 5 down
rclick() { click "$1" "$2" 3; }
dblclick() { move "$1" "$2"; x xdotool click --repeat 2 --delay 80 1; }
key()    { x xdotool key --clearmodifiers "$@"; }
type_()  { x xdotool type --delay 40 -- "$1"; }
# drag x1 y1 x2 y2 [steps] : press, move in steps, release (fires pointer events a human would)
drag() {
  local x1=$1 y1=$2 x2=$3 y2=$4 n=${5:-12} i
  move "$x1" "$y1"; settle 0.15
  x xdotool mousedown 1
  for ((i=1;i<=n;i++)); do
    move $((x1+(x2-x1)*i/n)) $((y1+(y2-y1)*i/n)); sleep 0.03
  done
  settle 0.15; x xdotool mouseup 1
}

# Held drag, for capturing overlays: dragstart X Y; dragto X Y [steps]; ...shot...; dragend
DRAG_X=0; DRAG_Y=0
dragstart() { DRAG_X=$1; DRAG_Y=$2; move "$1" "$2"; settle 0.25; x xdotool mousedown 1; sleep 0.1; }
dragto() {
  local n=${3:-12} i
  for ((i=1;i<=n;i++)); do move $((DRAG_X+($1-DRAG_X)*i/n)) $((DRAG_Y+($2-DRAG_Y)*i/n)); sleep 0.03; done
  DRAG_X=$1; DRAG_Y=$2; settle 0.3
}
dragend() { x xdotool mouseup 1; settle 0.5; }

INDEX="$PARITY_OUT/index.tsv"
[ -f "$INDEX" ] || printf 'name\tfile\tkind\tnote\n' > "$INDEX"

# shot <name> [note] : client-area PNG at 1x
shot() {
  settle
  x bash -c "import -window $WID /tmp/parity-shot.png"
  docker cp "$PARITY_CONTAINER:/tmp/parity-shot.png" "$PARITY_OUT/$1.png" >/dev/null
  printf '%s\t%s\tshot\t%s\n' "$1" "$1.png" "${2:-}" >> "$INDEX"
  echo "shot $1"
}

# shot_root <name> [note] : whole desktop (needed for native OS menus, which are separate X windows)
shot_root() {
  settle
  x bash -c "import -window root /tmp/parity-shot.png"
  docker cp "$PARITY_CONTAINER:/tmp/parity-shot.png" "$PARITY_OUT/$1.png" >/dev/null
  printf '%s\t%s\troot\t%s\n' "$1" "$1.png" "${2:-}" >> "$INDEX"
  echo "root $1"
}

# record <name> <seconds> <fps> -- <command...> : record the window while the command runs,
# then split to numbered PNG frames at exactly <fps>. Needs ffmpeg in the container.
record() {
  local name=$1 secs=$2 fps=$3; shift 4
  eval "$(x xdotool getwindowgeometry --shell "$WID")"
  x bash -c "rm -rf /tmp/pf && mkdir -p /tmp/pf"
  # window position on the root: x11grab needs absolute coordinates of the client area
  local ox oy
  eval "$(x xwininfo -id "$WID" | awk '/Absolute upper-left X/{print "ox="$4} /Absolute upper-left Y/{print "oy="$4}')"
  docker exec -d -e DISPLAY="$PARITY_DISPLAY" "$PARITY_CONTAINER" bash -c \
    "ffmpeg -loglevel error -y -f x11grab -framerate $fps -video_size ${PARITY_W}x${PARITY_H} -i ${PARITY_DISPLAY}+${ox},${oy} -t $secs /tmp/pf/rec.mkv"
  sleep 0.4
  "$@"
  sleep "$secs"; sleep 1
  x bash -c "cd /tmp/pf && ffmpeg -loglevel error -i rec.mkv -vf fps=$fps f-%04d.png"
  mkdir -p "$PARITY_OUT/frames/$name"
  docker cp "$PARITY_CONTAINER:/tmp/pf/." "$PARITY_OUT/frames/$name/" >/dev/null
  printf '%s\tframes/%s\tframes\t%s fps, %s s (frame 1 = start of recording)\n' "$name" "$name" "$fps" "$secs" >> "$INDEX"
  echo "frames $name"
}

# js <code> : reference app only, evaluates in the webview over the Tauri MCP bridge (:9223).
js() {
  docker cp "$(dirname "${BASH_SOURCE[0]}")/bridge-eval.mjs" "$PARITY_CONTAINER:/tmp/bridge-eval.mjs" >/dev/null
  x bun /tmp/bridge-eval.mjs "${PARITY_BRIDGE_PORT:-9223}" "$1"
}
