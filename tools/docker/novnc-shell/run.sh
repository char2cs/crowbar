#!/usr/bin/env bash
# Open, unauthenticated desktop on every host interface: anyone who can reach the port gets it.
# No bind mounts, no docker socket, no host network; state lives in the container's own filesystem.
set -euo pipefail
docker rm -f crowbar-novnc-shell >/dev/null 2>&1 || true
exec docker run -d --init --name crowbar-novnc-shell --restart unless-stopped \
  --shm-size 1g -p 0.0.0.0:6090:6080 crowbar-novnc-shell:latest
