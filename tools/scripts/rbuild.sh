#!/usr/bin/env bash
# Run a command inside the shared Rust builder container.
#
# The builder keeps cargo's target directory in RAM (tmpfs) so builds do not hit the disk, mounts
# this worktree at /w and the cargo registry from a named volume, and serialises callers with a
# lock. Usage: tools/scripts/rbuild.sh cargo test -p layout
#        tools/scripts/rbuild.sh --reset   # recreate the container (empties the RAM target)
# Each builder has its own lock, so builders in separate worktrees build at the same time.
# RBUILD_TARGET_VOLUME=<name> keeps the target in a named docker volume on disk instead of RAM,
# so more builders fit in memory.
# Builders share compiled crates through sccache when the RBUILD_SCCACHE volume holds
# bin/sccache; without it they build as before.
set -euo pipefail

name=${RBUILD_NAME:-crowbar-build}
image=${RBUILD_IMAGE:-a-gpui}
registry_volume=${RBUILD_REGISTRY:-a-cargo}
target_size=${RBUILD_TARGET_SIZE:-10g}
lock=${RBUILD_LOCK:-/tmp/claude-1000/cargo-${name}.lock}
target_volume=${RBUILD_TARGET_VOLUME:-}
sccache_volume=${RBUILD_SCCACHE:-crowbar-sccache}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

create_builder() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  local target_mount=(--tmpfs "/target:rw,exec,size=${target_size}")
  if [[ -n $target_volume ]]; then
    target_mount=(-v "$target_volume:/target")
  fi
  local cache=()
  if docker run --rm -v "$sccache_volume:/sccache" "$image" test -x /sccache/bin/sccache 2>/dev/null; then
    cache=(-v "$sccache_volume:/sccache" -e RUSTC_WRAPPER=/sccache/bin/sccache
      -e SCCACHE_DIR=/sccache/cache -e SCCACHE_CACHE_SIZE=12G)
  fi
  docker run -d --init --name "$name" \
    --ulimit core=0 \
    "${target_mount[@]}" \
    "${cache[@]}" \
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

needs_lock=1
case "${1:-} ${2:-}" in
  "cargo fmt" | "rustfmt "*) needs_lock=0 ;;
esac
if ((needs_lock)); then
  exec 9>"$lock"
  flock 9
fi
if [[ $(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null || echo false) != true ]]; then
  create_builder
fi
exec docker exec -e CARGO_BUILD_JOBS="${RBUILD_JOBS:-4}" -w "${RBUILD_CWD:-/w/app}" "$name" "$@"
