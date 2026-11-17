#!/usr/bin/env bash
# Stage the binaries and build the image. Usage: build.sh <dir holding crowbar crowbar-api panes/>
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
src=${1:?usage: build.sh <dir with crowbar crowbar-api panes>}
ctx=$(mktemp -d)
trap 'rm -rf "$ctx"' EXIT
cp -r "$here"/{Dockerfile,entrypoint.sh,index.html,rc.xml} "$src"/{crowbar,crowbar-api,panes} "$ctx"/
docker build -t crowbar-novnc-shell:latest "$ctx"
