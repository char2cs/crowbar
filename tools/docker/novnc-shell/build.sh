#!/usr/bin/env bash
# Builds the side-by-side rig image crowbar-novnc-shell:latest. See README.md.
#   build.sh                 compile everything, then build the image
#   build.sh --stage <dir>   build the image from artefacts already compiled into <dir>
# Compiles: the Tauri .deb + its web from origin/develop and the daemon, seeders and panes from HEAD
# (in a throwaway desksplit-rust container), and the Rust shell (Release) in the shared crowbar-build
# container via tools/scripts/rbuild.sh. All intermediate trees and caches are deleted afterwards.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(git -C "$here" rev-parse --show-toplevel)
image=${RIG_IMAGE:-crowbar-novnc-shell:latest}
builder=${RIG_BUILDER_IMAGE:-desksplit-rust:latest}
develop=${RIG_DEVELOP_REF:-origin/develop}
work=$(mktemp -d "${TMPDIR:-/tmp}/crowbar-rig.XXXXXX")
compiled=
cleanup() {
  # The builder writes as uid 1000 inside the container; remove through a container too.
  if [[ -n $compiled ]]; then
    docker run --rm -v "$work:/w" alpine rm -rf /w/develop /w/branch /w/cache >/dev/null 2>&1 || true
    docker exec crowbar-build rm -rf /tmp/rig-target >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

compile() {
  local out=$work/stage
  compiled=1
  mkdir -p "$work"/{develop,branch,stage} "$work"/cache/{gomod,gocache,bun,cargo-registry,cargo-git}
  git -C "$repo" archive "$develop" desktop web | tar -x -C "$work/develop"
  git -C "$repo" archive HEAD api app/panes | tar -x -C "$work/branch"
  docker run --rm --user 1000:1000 -e HOME=/home/dev -e CARGO_BUILD_JOBS="${RIG_JOBS:-3}" \
    -v "$work:/rig" \
    -v "$work/cache/gomod:/home/dev/go/pkg/mod" -v "$work/cache/gocache:/home/dev/.cache/go-build" \
    -v "$work/cache/bun:/home/dev/.bun/install/cache" \
    -v "$work/cache/cargo-registry:/home/dev/.cargo/registry" -v "$work/cache/cargo-git:/home/dev/.cargo/git" \
    --entrypoint bash "$builder" -lc '
set -euo pipefail
cd /rig/branch/api
export CGO_ENABLED=0
go build -tags noEmbed -o /rig/stage/crowbar-api ./cmd/crowbar
go build -tags noEmbed -o /rig/stage/crowbar-seed ./cmd/crowbar-seed
go build -tags noEmbed -o /rig/stage/crowbar-seed-chat ./cmd/crowbar-seed-chat
cd /rig/branch/app/panes
bun install --frozen-lockfile && bun run build:panes && cp -r dist-panes /rig/stage/panes
cd /rig/develop/web
bun install --frozen-lockfile && VITE_API_URL= NODE_OPTIONS=--max-old-space-size=4096 bun run build
cd /rig/develop/desktop/src-tauri
mkdir -p binaries && cp /rig/stage/crowbar-api binaries/crowbar-api-x86_64-unknown-linux-gnu
bunx @tauri-apps/cli@2.12.4 build --bundles deb \
  --config "{\"build\":{\"beforeBuildCommand\":\"\"},\"bundle\":{\"macOS\":{\"files\":{}}}}"
cp target/release/bundle/deb/*.deb /rig/stage/crowbar-desktop.deb'
  "$repo/tools/scripts/rbuild.sh" env CARGO_TARGET_DIR=/tmp/rig-target \
    cargo build --release -p crowbar --bin crowbar
  docker cp crowbar-build:/tmp/rig-target/release/crowbar "$out/crowbar"
}

if [[ ${1:-} == --stage ]]; then
  stage=$(cd "${2:?--stage <dir>}" && pwd)
else
  compile
  stage=$work/stage
fi
ctx=$work/context
mkdir -p "$ctx/stage"
cp -r "$here"/{Dockerfile,entrypoint.sh,index.html,README.md,rig,agent,forge,seed} "$ctx"/
cp -r "$stage"/{crowbar,crowbar-api,crowbar-seed,crowbar-seed-chat,panes,crowbar-desktop.deb} "$ctx/stage"/
docker build -t "$image" "$ctx"
