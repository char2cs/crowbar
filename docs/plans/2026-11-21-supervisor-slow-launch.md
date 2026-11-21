# Supervisor: a slow launch stays running, as develop's sidecar does

## Problem

When a launch is not healthy within its startup budget (30 polls, 200ms apart), the Rust
supervisor stops the child and charges a death (`after_startup`, `Startup::NeverHealthy` in
`crates/supervisor/src/supervisor/supervise.rs`). Develop's sidecar does neither:
`desktop/src-tauri/src/sidecar/mod.rs` `spawn` logs "did not become healthy" and leaves the
child running. Only a real exit reaches `handle_termination` and the restart budget, and the
watchdog (`sidecar/watchdog.rs`) kills an unserved child only after 6 failed probes and its
wedge checks.

On the rig, stopping the shared daemon for about 20s spent Rust's budget (3 respawns in
10 minutes) and raised the crash panel. Tauri kept its one respawned child, which served
again once the daemon came back.

## Behaviour after the change

1. A launch that misses the startup budget keeps running. It is not stopped and not charged.
2. Only an exit charges the budget. That includes an exit the watchdog causes by killing a
   wedge. A spawn error (binary missing) is still charged on purpose, so the user gets the
   crash panel. Develop logs it and leaves a silent screen
   (`desktop/src-tauri/src/sidecar/mod.rs:258`); the audit row records this difference.
3. The watchdog watches the child from the moment the startup budget runs out. It uses
   develop's rule: 6 consecutive unserved probes, then the existing suspended, answering and
   busy exonerations, then SIGQUIT and SIGKILL. A held (halted) answer never counts.
4. A late `Ready` comes from the supervisor's own state, prompted by events rather than a
   tick. Two signals feed it, and whichever comes first wins:
   - a watchdog probe that answers healthy;
   - the first answer the shell's own stream sees from this computer's daemon (the store
     link turning Live). The shell reports it with `Supervisor::heard_answer()`.
   While unready, `monitor` handles either by running one health check right away. A
   healthy answer goes through the pid check from `become_ready`, then `Ready { pid }` is
   emitted once. `current()` stays the single owner, and there is no timer or flag.
5. The startup machine and the pty readiness gate keep waiting on `current()` for `Ready`,
   unchanged. While unready, `current()` stays `Starting` or `Restarted`, so the screen keeps
   saying the backend is starting. The backend toasts already follow the store's link, so
   they recover as soon as the stream reconnects, as develop's do.

## Shape

- `Startup::NeverHealthy` leads to `watch_unready(supervision, child, commands)`. It starts
  the watchdog and enters the same `monitor` loop as a ready child: an exit means Recover,
  a wedge means kill then Recover, and stop or restart commands are handled as today.
  `become_ready` keeps its pid check and its `Ready`.
- The watchdog sends healthy outcomes to the supervision loop on a new channel, and
  `Command::Answered` carries the shell's report. While `current()` is not `Ready`,
  `monitor` answers either with one health check, the pid check and `Ready`.
- No new timings. `Ready` follows the first healthy signal, so it does not wait for the
  watchdog's interval.

## Tests (red first, through `Supervisor`'s public API with a fake daemon)

- A child that stays unserved past the startup budget is still running, and no `Recovering`
  is sent.
- That child answering later, reported through `heard_answer()`, yields one `Ready { pid }`
  well before `watchdog_interval` (a long interval in the test).
- A watchdog probe that finds the child healthy also yields one `Ready`, and a second
  signal adds nothing.
- That child dying yields `Recovering` with the exit reason, charged once.
- Three such deaths in the window, then a fourth, yield `GaveUp`. Unserved time alone never
  does.
- A child that never answers is killed after `failure_threshold` probes, `Unhealthy` is sent
  first, and the exit is charged.

## Out of scope

- The 30-poll startup budget, the backoff and the window are unchanged.
- `Halted` is unchanged.
