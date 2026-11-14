#!/usr/bin/env bash
# Usage: run-scenario.sh <reference|candidate> <scenario> <out-dir>
# Runs scenarios/<scenario>.sh against the app already launched in the container.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
target=${1:?reference|candidate}; scenario=${2:?scenario name}; out=${3:?out dir}
case "$target" in
  reference) export PARITY_CONTAINER=${PARITY_CONTAINER:-inventory-app} PARITY_WINDOW=${PARITY_WINDOW:-Crowbar} ;;
  candidate) export PARITY_CONTAINER=${PARITY_CONTAINER:?candidate needs PARITY_CONTAINER} PARITY_WINDOW=${PARITY_WINDOW:-Crowbar} ;;
  *) echo "target must be reference or candidate" >&2; exit 2 ;;
esac
export PARITY_TARGET=$target PARITY_OUT="$out/$target/$scenario"
source "$here/lib.sh"
attach
source "$here/scenarios/$scenario.sh"
echo "wrote $PARITY_OUT"
