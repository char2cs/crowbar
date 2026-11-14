#!/usr/bin/env bash
# Usage: contact-sheet.sh <dir-with-pngs> <out.png> [pattern] [crop WxH+X+Y] [tile COLSxROWS] [thumb WxH]
# Builds a labelled contact sheet via ImageMagick inside the desktop container.
set -euo pipefail
c=${PARITY_CONTAINER:-inventory-app}
dir=$1; out=$2; pat=${3:-*.png}; crop=${4:-1200x800+0+0}; tile=${5:-4x}; thumb=${6:-600x400}
docker exec "$c" rm -rf /tmp/cs && docker exec "$c" mkdir -p /tmp/cs
docker cp "$dir/." "$c:/tmp/cs/"
docker exec "$c" bash -c "cd /tmp/cs && montage $pat -crop $crop +repage -tile $tile -geometry $thumb+4+4 -background '#888' /tmp/cs-out.png"
docker cp "$c:/tmp/cs-out.png" "$out"
