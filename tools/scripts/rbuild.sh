#!/usr/bin/env bash
# Run a command inside the shared Rust builder container.
#
# The builder keeps cargo's target directory in RAM (tmpfs) so builds do not hit the disk, mounts
# this worktree at /w and the cargo registry from a named volume, and serialises callers with a
# lock. Usage: tools/scripts/rbuild.sh cargo test -p model
#        tools/scripts/rbuild.sh --reset   # recreate the container (empties the RAM target)
set -euo pipefail

name=${RBUILD_NAME:-crowbar-build}
image=${RBUILD_IMAGE:-a-gpui}
registry_volume=${RBUILD_REGISTRY:-a-cargo}
target_size=${RBUILD_TARGET_SIZE:-8g}
lock=${RBUILD_LOCK:-/tmp/claude-1000/cargo.lock}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

create_builder() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run -d --name "$name" \
    --tmpfs "/target:rw,exec,size=${target_size}" \
    -v "$registry_volume:/usr/local/cargo/registry" \
    -v "$root:/w" \
    -e CARGO_TARGET_DIR=/target \
    -e CARGO_BUILD_JOBS=2 \
    -e CARGO_PROFILE_DEV_DEBUG=0 \
    -e CARGO_INCREMENTAL=0 \
    -e ZED_ALLOW_EMULATED_GPU=1 \
    -w /w/app \
    "$image" sleep infinity >/dev/null
}

if [[ ${1:-} == --reset ]]; then
  exec 9>"$lock"
  flock 9
  create_builder
  exit 0
fi

(($# > 0)) || { echo "usage: rbuild.sh <command...> | --reset" >&2; exit 2; }

exec 9>"$lock"
flock 9
if [[ $(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null || echo false) != true ]]; then
  create_builder
fi
exec docker exec -w "${RBUILD_CWD:-/w/app}" "$name" "$@"
