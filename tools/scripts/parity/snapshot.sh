#!/usr/bin/env bash
# Save or restore the reference app's whole state (daemon home + webview storage) so every
# scenario starts from identical data. Stops and restarts the app (launch script in the container).
# Usage: snapshot.sh save|restore [name]      (default name: golden)
set -euo pipefail
c=${PARITY_CONTAINER:-inventory-app}; op=${1:?save|restore}; name=${2:-golden}
here=$(cd "$(dirname "$0")" && pwd)
docker exec "$c" bash -c 'pkill -x crowbar-desktop || true; sleep 1; pkill -x crowbar-api || true; sleep 1'
case "$op" in
  save) docker exec "$c" bash -c "tar -C / --exclude=WebKitCache -czf /root/snap-$name.tgz root/data/home root/.local/share/software.rabbyte.crowbar" ;;
  restore) docker exec "$c" bash -c "rm -rf /root/data/home /root/.local/share/software.rabbyte.crowbar && tar -C / -xzf /root/snap-$name.tgz" ;;
  *) echo "save|restore" >&2; exit 2 ;;
esac
docker exec -d "$c" bash -c '/root/launch-ref.sh > /root/data/app2.log 2>&1'
sleep 25
export PARITY_CONTAINER=$c PARITY_OUT=${PARITY_OUT:-/tmp/parity-reset}; mkdir -p "$PARITY_OUT"
source "$here/lib.sh"; attach; sleep 3
