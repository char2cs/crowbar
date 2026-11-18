# supervisor

## Owns

The daemon sidecar's life: locating the binary, spawning it, waiting for its health and
migration readiness, the watchdog, recovery and restart budget, stopping it, and the daemon log
it captures (with a crash report when the daemon dies).

## Public interface

- `Supervisor`: starts, supervises and stops the daemon; `SupervisorConfig`, `Timings`, `DaemonPaths`, `locate_binary`.
- `DaemonEvent` and `MigrationProgress`: what the shell is told while the daemon starts and runs.
- `fetch`: one HTTP request over the daemon socket.
- `DaemonLog`, `Health`, `parse_health`, `wait_for_health`, `Probe`, `probe_ready`.
- `Error`.

## Depends on

No workspace crate (tokio, hyper, rustix).
