# Terminal Session Persistence Design

**Date:** 2026-06-27 (revised 2026-06-28 after adversarial spec review)
**Status:** Approved for implementation — phased
**Branch:** hardening/production-readiness

## Problem

Terminal sessions in Crowbar die whenever the user switches workspaces. On a switch, `destroyWorkspaceStore` calls `killTerminalSession` for every terminal, which issues `DELETE /…/terminals/:id` and kills the PTY process. Running commands, scrollback, and working directory are all lost, and the user must re-orient on every context switch. This is a showstopper.

Goal: terminal sessions survive workspace switches (process kept alive), and survive app close / crash / machine restart as far as the OS allows (scrollback + working directory restored; a still-running process cannot survive a process-tree teardown — see *What This Does NOT Solve*). Implemented entirely in the Go daemon — no external dependency such as tmux.

## What Already Exists (grounding)

The original draft of this spec proposed building a ring buffer and decoupling the PTY from the WebSocket. **That work is already done and unit-tested** — the review corrected this. Before writing any backend code, note the current state of `api/internal/engine/terminal/`:

- `internal/session/ring.go` — a thread-safe `RingBuffer` (`Write`, `Snapshot`), `defaultRingSize = 64KB`. Tested (`ring_test.go`: WrapAround, Overflow, ConcurrentWrites, SnapshotIsChronological).
- `internal/session/session.go` — `Session` already holds `ring *RingBuffer` and `clients map[*client]struct{}`. The PTY pump writes to the ring **then** fans out to clients (`s.ring.Write(chunk)` then `s.fanOut(chunk)`). `Attach() (<-chan OutputFrame, error)` snapshots the ring and registers a client under `s.mu`; `Detach(ch)` removes one client. Multi-client fan-out is a deliberate, documented capability.
- `terminal.go` — engine `Attach(ctx, sid, conn)` is a **blocking** call: it calls `s.Attach()`, runs the read/write pumps, and on WS close calls `s.Detach(ch)` (≈ terminal.go:265) while the PTY keeps running. The session is removed from the registry only on `Kill()`, `Shutdown()`, or PTY self-exit (`reapOnDone`).

Consequences for the design:
- **The PTY already survives a WebSocket drop.** A workspace switch only needs the frontend to *stop killing* the session and to close + later re-open the WebSocket.
- The remaining backend work is **lifecycle state, idle detection, disk persistence, restore, and wire-protocol/limits** — not the ring or the decouple.
- Raising the ring from 64KB to a larger default is a **resize**, not a new buffer, and must be justified against the per-session memory budget (see *Session Limits*).

## Phasing (value/risk ordered)

The feature decomposes into three independently shippable phases. Earlier phases deliver the showstopper fix with minimal risk; later phases add durability across process death.

**Phase 1 — Survive workspace switch (highest value, lowest risk).** No disk persistence, no GORM table, no IndexedDB index, no launchd. The daemon is alive whenever the app is open (it is a Tauri sidecar), so a switch only requires: stop the frontend kill, detach (close the WS without `DELETE`), and on re-entry re-attach to the still-live PTY discovered via the existing `ListSessionsForWorkspace`. The ring replay already restores scrollback.

**Phase 2 — Idle suspend / restore within a running daemon.** Adds the `active|detached|suspended|exited` lifecycle, idle detection, per-workspace soft limit + global hard ceiling, suspend-to-disk and restore, the wire-protocol status extension, and concurrency hardening. Because suspend/restore must persist and reload session state, Phase 2 also introduces the durable store (global `view.db` `TerminalSession` rows) and the DI wiring (`SessionMetaStore` port + storage-path resolution). Bounds memory for long-lived detached sessions.

**Phase 3 — Persistence across daemon restart.** Builds on Phase 2's store: reconcile-on-open registry repopulation (PTY-less placeholders), restore-on-first-attach after restart, and graceful-shutdown flush (replace the Tauri SIGKILL-on-close with SIGTERM). Restart-restore works via the existing sidecar-on-app-open path; **launchd / a boot-time background daemon is explicitly deferred** (see *Daemon Lifecycle*).

Each phase has its own acceptance criteria (see *Testing & Acceptance*).

## Session Lifecycle

States and transitions:

```
                 last client WS closes (no DELETE)
   ┌──────────┐  ───────────────────────────────►  ┌────────────┐
   │  Active  │                                      │  Detached  │
   │ clients  │  ◄───────────────────────────────   │ clients==0 │
   │   > 0    │        client re-attaches (WS)        │ PTY alive  │
   └──────────┘                                      └────────────┘
        │                                              │        │
        │ PTY exits / user `exit` / Kill               │        │ idle + over limit
        ▼                                              │        ▼
   ┌──────────┐                                        │   ┌────────────┐
   │  Exited  │  ◄─────────────────────────────────────┘   │ Suspended  │
   │ terminal │     PTY exits while detached                │ PTY killed │
   └──────────┘                                            └────────────┘
                                                          re-attach │
                                                  (fresh shell + replay)
                                                                    ▼
                                                               Active/Detached
```

- **Active** — at least one client WebSocket is attached. *Active is not a stored scalar; it is derived under `s.mu` as `len(clients) > 0`.* WS connected, xterm mounted, PTY running, ring in memory.
- **Detached** — the last client WS closed (workspace switch). PTY keeps running; output continues into the ring. `lastActiveAt` recorded. A session with a **running foreground process is never auto-suspended** — it stays Detached indefinitely.
- **Suspended** — a Detached, zero-client, **idle** session (no running foreground process) that its workspace's soft limit (or the global ceiling) selected for suspension. The PTY is killed *intentionally* (distinct from termination — see issue-driven note below), scrollback flushed to disk, CWD captured. On re-attach a fresh shell spawns **under the same session id** in the saved CWD and the scrollback replays.
- **Exited** (terminal) — the PTY exited on its own (`reapOnDone`) or via explicit `Kill`/user `exit`. The session is terminated, **not** suspended: its persistence (`.buf` file + `view.db` row) is deleted and an `ended` lifecycle frame (with `exitCode` when available) instructs the frontend to prune the session id. Exited sessions are never restored.

**Suspend must be distinct from termination.** Today every PTY death runs `reapOnDone` → `reg.Remove` + `fireEnded` (an `ended` frame) — indistinguishable from a real exit, and it removes the registry entry so a later attach 404s. Suspend therefore sets a `suspending` flag (under `s.mu`) before tearing down the PTY; `reapOnDone` checks it and, when set, **keeps the registry entry**, transitions state to `suspended`, and **suppresses `fireEnded`**. `Restore` re-registers under the **same** session id (refactor `Create` to delegate to a private `spawn(id, …)` so `Restore` can pass the persisted id rather than minting a new UUID).

**Machine-restart / post-crash classification.** After a daemon restart the registry is empty and every PTY is dead. On start, the daemon repopulates the registry from the durable store as **suspended placeholders** (PTY-less). A session persisted as `active` (daemon crashed before it could detach) or `detached` is treated identically to a suspended session on restore: PTY gone, scrollback + CWD loaded, fresh shell on re-attach. A running process cannot survive this — acknowledged in *What This Does NOT Solve*.

**Placeholder representation & restore-aware attach.** Today `session.New` unconditionally `pty.Start`s and `Session` has no liveness concept, and engine `Attach` is a pure `reg.Get → s.Attach()` passthrough with no restore branch — so neither a suspended session nor a placeholder is representable or reattachable. Add to `Session` a `state` (and a `ptmx == nil` sentinel) with an `IsLive() bool` accessor; add a `NewPlaceholder(id, meta, scrollback)` constructor that builds a PTY-less session (no pump) with the ring pre-loaded from `.buf` and `state = suspended`. Make engine `Attach(ctx, sid, conn)` **restore-aware**: after `reg.Get`, if `!s.IsLive()`, call `Restore(sid)` (spawn the PTY under the same id via `spawn(id, savedCwd, shell)`, load `.buf` into the ring, start the pump, transition state) **before** `s.Attach()`. `Restore` is guarded by `s.mu` and idempotent so concurrent first-attaches don't double-spawn. `SessionExists` must count a placeholder as existing (so the WS upgrade does **not** 404 — this is what makes reconnect-after-restart reachable); `Write`/`Resize` against a not-yet-live session either trigger `Restore` or return a typed not-live error rather than silently no-opping.

## Architecture

### Ring buffer (reuse, resize, fix the handoff)

Reuse `internal/session/ring.go`. Add two methods for persistence: `Flush(io.Writer) error` and `Load(io.Reader) error`. **Do not create a second ring package.**

Raising the default from 64KB to a larger value is a memory-budget decision: rings are allocated eagerly (`make([]byte, capacity)`), so each session reserves its full ring immediately. Default chosen: **256KB** per session (4× current, not 32×), configurable. The binding constraint is the **100-session count ceiling** (see *Session Limits*): 100 × 256KB = **25.6MB** of rings, well under the 256MB global byte ceiling — the byte ceiling is a soft safety bound that only bites if the count cap is mis-configured, not the active limit.

**Replay→live handoff invariant (fixes a real duplication race).** Today the pump appends to the ring under `ring.mu` and fans out under `s.mu` as *two* critical sections, while `Attach` snapshots+registers under `s.mu`. A chunk written to the ring just before a client registers but fanned out just after is delivered **twice** (once in the snapshot, once live), corrupting xterm. Fix: the pump must hold `s.mu` across **both** `ring.Write(chunk)` and `fanOut(chunk)` (lock order `s.mu → ring.mu`, matching `Attach`). Then any attach sees a chunk either only in the snapshot or only live — never both. Covered by a `-race` regression test (see *Testing*).

### Working-directory tracking

Restore must land in the directory the user was in, not the launch directory. The daemon has no live CWD today and macOS has no `/proc`.

**The API is built `CGO_ENABLED=0`** (hardcoded in `docker/Dockerfile` and both release workflows; pure-Go SQLite `glebarez/sqlite` exists precisely to keep this posture so one runner can cross-compile linux/{amd64,arm64} + darwin/{amd64,arm64} + windows/amd64). **Adding `import "C"` is not acceptable** — it forces `CGO_ENABLED=1` and breaks the release matrix. So `proc_pidinfo` (a cgo call) is **not** the primary mechanism. CWD capture, in resolution order:

1. **OSC 7 scan in the pump (primary).** The pump scans output chunks (the same bytes written to the ring) for OSC 7 (`ESC ] 7 ; file://host/path BEL`) and updates a mutable `session.cwd` — the daemon-side equivalent of the existing frontend `parseOSC7`. OSC 7 emission depends on shell config, so this is best-effort.
2. **Pure-Go `proc_info` (optional, darwin-only).** If accurate live CWD is wanted when OSC 7 is absent, resolve the shell PID's CWD via the macOS `proc_info` syscall through `golang.org/x/sys/unix` (already a dependency, used in `lock_unix.go`) behind a `//go:build darwin` file — **no cgo**. If a pure-Go path proves infeasible, this fallback is dropped, not implemented via cgo.
3. **Create-time `cmd.Dir`** → workspace root.

Because the primary source is best-effort, the CWD-accuracy gap (shell emits no OSC 7) is documented in *What This Does NOT Solve*. `session.New` must also retain `shell`, `profileId`, and the mutable `cwd` (thread `profileId` through — it is currently dropped).

### Storage layout and DI

Scrollback is a raw file under the per-workspace storage dir; metadata is a row in the **global** `state/view.db`. There is **no `.meta` JSON file** (it would be a redundant, non-atomic third source of truth — see *Source of truth*).

```
~/.crowbar/
├── state/view.db                         # global GORM DB — TerminalSession rows (authoritative metadata)
└── projects/<projectId>/<repoId>/workspaces/<wsId>/storages/
    └── terminal_sessions/
        └── <sessionId>.buf               # scrollback ring snapshot (raw bytes, atomically written)
```

For **home workspaces** `RepoID` is empty, so the path collapses to `projects/<projectId>/workspaces/<wsId>/storages/terminal_sessions/` (via `filepath.Join`). **Home `WorktreePath` is the user's real project directory and must never be used to derive the storage dir** — always resolve via `worktreepath.StorageDir(crowbarHome, projectID, repoID, wsID)`.

**Dependency wiring (currently absent).** The engine is built with `engineterminal.New()` (no args), holds no GORM/repo handle, and its only outward hook (`OnSessionEnded`) carries just `(ctx, workspaceID, sessionID)`. `Create` receives only the *worktree* path. So engine-internal transitions (`Detach`, `Suspend`, `reapOnDone`, `Shutdown`) **cannot write `view.db` or resolve a storage dir** as they stand. Two injected dependencies fix this:

1. **`SessionMetaStore` port.** Define a small interface in the engine package — `Save(ctx, TerminalSessionMeta) error` and `Delete(ctx, sessionID) error`, where `TerminalSessionMeta` carries `{sessionId, workspaceId, projectId, repoId, cwd, shell, profileId, state, lastActiveAt}`. The **terminal usecase** (`usecases/container.go`) implements it over the `state/view.db` `TerminalSession` GORM store and injects it via `engineterminal.New(metaStore, …)` (mirroring how `OnSessionEnded` is already wired). Engine code calls `metaStore.Save` on create/detach/suspend/shutdown and `metaStore.Delete` on reap/exit/Kill. The engine never imports GORM.
2. **Storage-path resolution.** The usecase resolves `wsID → (projectID, repoID)` via the global `workspace_locations` index, computes the storage dir via `worktreepath.StorageDir(crowbarHome, projectID, repoID, wsID)`, and passes the resolved path into `Create` (stored per session) so the suspend/flush goroutines know where to write the `.buf`.

`LoadPersistedSessions` lives in the usecase (which can reach the index + store): it enumerates `TerminalSession` rows (which carry `projectID`/`repoID`), resolves each storage dir, and registers suspended placeholders — **never** a global `projects/*/*/workspaces/*/storages` filesystem walk.

### Source of truth & crash consistency

- **Metadata** (`sessionId`, `workspaceId`, `projectId`, `repoId`, `cwd`, `shell`, `profileId`, `state`, timestamps) lives **only** in the global `state/view.db` `TerminalSession` row — the same plain-CRUD GORM database as `terminal_profiles` (SQLite WAL/ACID, no torn writes). This **supersedes decision D6** ("terminals are ephemeral; no `terminal_sessions` view.db") — see *Superseded Decisions*.
- **Scrollback** (`.buf`) is written via a hardened atomic write: `os.CreateTemp(dir, sessionId+".buf-*")` (a **unique** temp name per flush — never a fixed `.tmp`, so concurrent flushes can't interleave into one file), write the snapshot, **`tmp.Sync()`**, close, `os.Rename` over `<sessionId>.buf`, then **fsync the parent directory**. The temp+rename skeleton mirrors `writeAtomic` in `engine/search/internal/replace/replace.go`, **but that helper performs no `fsync` at all** — both syncs above are additions here and are load-bearing: without them a power-loss rename can resolve ahead of the data, yielding a zero-length/torn `.buf`. An interrupted flush leaves the previous good `.buf` intact, so at most the last flush-interval of scrollback is lost, never the whole file.
- **Authority on conflict:** `view.db` is authoritative for metadata; the `.buf` is authoritative for scrollback. On daemon start, **reconcile-on-open** (mirroring `repositories/workspace/internal/store/store.go`): enumerate `TerminalSession` rows, register each as a suspended placeholder, drop rows whose `.buf` is missing/corrupt (log + skip, never block startup). A corrupt `.buf` falls back to empty scrollback; the metadata row still drives CWD/shell restore.

### Concurrency & lifecycle locking

A single per-session state machine guarded by the existing `Session.mu`:

- **Derived attachment state.** `active` iff `len(clients) > 0`, evaluated under `s.mu`. Add `AttachedCount() int`. The transition to `detached` fires only when the **last** client is removed (`len(clients) == 0`); persist `lastActiveAt` there.
- **Suspend re-verifies under the lock.** The auto-suspend sweep must, under `s.mu` immediately before killing, re-confirm `len(clients) == 0 && !suspending && IsIdle()`. This closes the TOCTOU window where a client re-attaches and launches a process between the idle poll and the kill — otherwise suspend could destroy a PTY now in active use, the exact failure the feature exists to prevent. Drop the proposed engine-level `Detach(ctx, sessionID)` — per-client detach already happens inside the blocking `Attach`.
- **`IsIdle()` locking is non-reentrant-safe.** `sync.Mutex` is not reentrant, and the existing `Kill()` → `shutdown()` path re-acquires `s.mu` inside `once.Do` (≈ session.go:242) — so holding `s.mu` *across* `Kill` self-deadlocks. Discipline: add `isIdleLocked()` (does the `tcgetpgrp` ioctl on the encapsulated `ptmx` via the shell PID, **caller already holds `s.mu`**) and a public `IsIdle()` (acquires `s.mu`, calls `isIdleLocked`). The auto-suspend sweep calls `isIdleLocked` directly while it already holds `s.mu` for the re-verify (never the public `IsIdle`, which would re-lock). Modify `Kill()` to take `s.mu` **only around** `ptmx.Close()` / `Process.Kill()` and **release it before** calling `shutdown()`. This both prevents the ioctl from racing `ptmx.Close()` (EBADF / fd reuse) and avoids the self-deadlock. `ptmx`/`cmd` stay unexported. (`Resize` issues an fd ioctl too and should take `s.mu` for full fd-safety; at minimum `Kill` and the idle ioctl must mutually exclude.)
- **Flush goroutine has a stop signal.** The periodic flush loop selects on `s.Done()` (and exits on suspend) so it never leaks per terminated session: `select { case <-ticker.C: flush(); case <-s.Done(): return }`.
- **Flushes are serialized per session.** The four flush triggers (10s cadence, detach, suspend, graceful shutdown) run on different goroutines. A dedicated `flushMu` (separate from `s.mu`, to avoid blocking the pump) is held across the whole snapshot → temp-write → `fsync` → rename → dir-`fsync` sequence; the flush that acquires `flushMu` last both snapshots and renames last, so the newest snapshot wins and no two goroutines write `.buf` concurrently. The per-session cadence goroutine is stopped (via `s.Done()`) before `Terminal.Shutdown` runs its final flush, so the two never overlap.

### Session limits & observability

Terminology: **"idle"** means `IsIdle()` is true (no running foreground process). The eviction **candidate order** is by ascending `lastActiveAt` (oldest-inactive first), a field on every Detached session — distinct from the idle predicate.

- **Per-workspace soft limit:** 10 simultaneous Detached sessions. Over the limit → suspend the oldest-`lastActiveAt` **idle** Detached session in that workspace. A Detached session with a running process is never suspended.
- **Global hard ceiling:** max total sessions (default 100) **and** max total ring bytes (default 256MB), tracked O(1) on the single global registry map. When exceeded, evict the oldest-`lastActiveAt` **idle** Detached session across all workspaces. If no idle Detached candidate exists and we are still over budget, the last resort is to **`Kill`** (not suspend) the oldest-`lastActiveAt` Detached session — even one with a running process — writing a one-line notice into its scrollback first and flushing its `.buf`+row so it returns as Suspended on next attach. This keeps the running-process guarantee intact for *suspension* (we never suspend a running shell) while still making the hard ceiling enforceable; the kill-last-resort is the documented, bounded exception. (Running processes already cannot survive app close/restart — see *What This Does NOT Solve*.)
- **Observability:** surface live/detached/suspended counts and total ring bytes in the UI (a daemon status row in settings), since detached sessions hold memory with no visible terminal.

### Daemon lifecycle & shutdown

The daemon is a **Tauri sidecar**: spawned on app launch and **SIGKILL**ed on window close (`lib.rs` `CommandChild::kill`). SIGKILL bypasses the graceful path, and even the graceful path (`Container.Close → Terminal.Shutdown`) currently kills every PTY without flushing. Two required changes for Phase 3:

1. **Graceful shutdown flushes.** `Terminal.Shutdown()` must flush every session's ring + persist its `view.db` row **before** `Kill`. Replace the Tauri `child.kill()` on window close with a graceful SIGTERM + bounded await so `main.go`'s SIGTERM handler → `Container.Close` runs the flush. `Shutdown` must no longer unconditionally kill detached/suspended sessions; it suspends idle ones (flush + persist) and flushes the rest. The original kill-all existed to avoid orphaned dev servers holding ports — on a true machine shutdown those processes die anyway, but their scrollback/CWD is flushed first.
2. **Flush cadence covers all live states, scrollback *and* metadata.** Flush every live session (active and detached) on a fixed cadence so a hard crash / `kill -9` loses at most the cadence interval, not everything. Cadence: **every 10s**, for any session with un-flushed output, write the `.buf` atomically **and** upsert the session's `view.db` row (live `cwd` from the OSC 7 scan, `state`, `lastActiveAt`); plus on detach, on suspend, on graceful shutdown. The row is created at `spawn(id, …)` time (not lazily), so reconcile-on-open can find even an active-never-detached session that was `kill -9`'d. This closes the gap where a crashed active session would otherwise restore scrollback but land in the create-time directory — both scrollback and CWD are now bounded by the cadence interval. (`proc_info`/OSC 7 only update the in-memory `cwd`; the cadence is what makes that durable.)

**launchd is explicitly deferred.** It conflicts with the sidecar (two processes contend for the fixed `~/.crowbar/crowbar.sock` single-instance lock), it is macOS-only against a CI-built Windows/Linux target set, and it has no consent/uninstall story. Its only payoff is running the daemon *while the app is fully closed* (pre-warming). Machine-restart **restore** does not need it: on next app open the sidecar starts, `LoadPersistedSessions` runs, and sessions restore on first attach. If revisited later, it must use `SMAppService` (login-item, OS-managed, surfaced in System Settings), be opt-in via a Settings toggle, and have an uninstall path — tracked as a follow-up, not part of this design's implementation.

### Lifecycle wire protocol

The lifecycle DTO `Status` is currently `active|ended` and the frontend mirror (`web/src/lib/types.ts`) is a closed union; the workspace snapshot is built from the live registry and hardcodes `"active"` in three places (`snapshots.go`, `sessions.go` ×2). Suspended/detached sessions therefore have no data path to the UI. Changes:

1. Extend the status union end-to-end to `'active' | 'detached' | 'suspended' | 'ended'`, plus optional `exitCode?: number` for the Exited case (Go DTO `Status` doc + `web/src/lib/types.ts`).
2. Re-source the snapshot/list from the durable store + live registry **union**, emitting each session's real state instead of the hardcoded `"active"`.
3. Engine `Detach`/`Suspend`/`Restore` and the reap path push a lifecycle frame with the correct status via the existing broadcaster. Suspend pushes `suspended` (never `ended`); reap pushes `ended` (+ `exitCode`).

## Backend Changes (by phase)

**Phase 1 (minimal):**
- No engine changes strictly required to keep the PTY alive on switch (it already survives WS drop). Optionally record a derived `detached` timestamp for observability.
- Confirm `ListSessionsForWorkspace` returns live sessions for re-entry discovery (it does).

**Phase 2** (suspend/restore needs persistence + DI, so these land here, not Phase 3):
- **DI / persistence wiring (prerequisite):** add the `TerminalSession` GORM model to `state/view.db` (`SessionID`/`WorkspaceID`/`ProjectID`/`RepoID`/`CWD`/`Shell`/`ProfileID`/`State`/timestamps); implement the `SessionMetaStore` port in the terminal usecase and inject it via `engineterminal.New(metaStore, …)`; resolve+thread the per-workspace storage path into `Create` (via `worktreepath.StorageDir`).
- `internal/session/ring.go`: add `Flush(io.Writer)` / `Load(io.Reader)`; bump default to 256KB (configurable).
- `internal/session/session.go`: retain `shell`/`profileId`/mutable `cwd`; add `state` + `IsLive()` + `NewPlaceholder`; `AttachedCount()`; `isIdleLocked()`/`IsIdle()` (tcgetpgrp); `suspending` flag; `flushMu`; OSC 7 scan in the pump; hold `s.mu` across `ring.Write`+`fanOut`; `Kill` takes `s.mu` only around `ptmx.Close`/`Process.Kill` then releases before `shutdown()`; flush goroutine bound to `s.Done()`.
- `terminal.go`: refactor `Create` → private `spawn(id, …)`; make `Attach` restore-aware (`Restore` when `!IsLive`); add `Suspend` (re-verify under lock, set `suspending`, kill, flush + `metaStore.Save`), `Restore` (re-register same id, load `.buf`, reset state); auto-suspend sweep (10s, soft limit + global ceiling, evict idle by `lastActiveAt`, Kill-last-resort); make `reapOnDone` honor `suspending`; `metaStore.Delete` + `.buf` delete on real exit/Kill; capture `cmd.Wait()` exit code for the `ended` frame.
- Persistence helpers (hardened atomic `.buf` write/read with unique temp name + fsyncs).
- Cadence goroutine: every 10s flush `.buf` + `metaStore.Save` (cwd/state/lastActiveAt) for sessions with un-flushed output.
- Wire protocol: extend status; re-source snapshot/list from store∪registry; push frames on transitions.

**Phase 3:**
- `LoadPersistedSessions` in the usecase (enumerate `TerminalSession` rows; resolve storage dirs; register suspended placeholders; reconcile-on-open — drop rows whose `.buf` is gone/corrupt).
- `Terminal.Shutdown` flush-then-kill (no unconditional kill-all); flush-all on `Container.Close`.
- Tauri `lib.rs`: graceful SIGTERM on window close instead of SIGKILL (so `Container.Close` runs the flush).

## Frontend Changes (by phase)

A clarification the review surfaced: there are two terminal renderers. **Bottom-panel terminals** are managed by `TerminalHost` (parked offscreen across switches; disposed only when `removeSession` makes them `ptyGone`). **Pane/buffer terminals** render via `TerminalTab → XtermTerminal` directly and are disposed by React on unmount — they are *not* parked by `TerminalHost`, so the "purge the parked xterm" rationale does not apply to them. Both kinds, however, key their daemon transport by the **daemon `connectionId`**, not the frontend tab/buffer id — the round-1 reconnect bug.

**Phase 1:**
- **Persist the tab→daemon mapping (localStorage, not a DB).** The frontend tab id (`terminal-tab-${…}`) is not the daemon PTY id (the `connectionId` returned by create); the latter lives only in the non-persisted `useTerminalStore`. Persist `{ tabSessionId → daemonConnectionId }` per workspace in **localStorage** (keeps Phase 1 free of IndexedDB). This mapping is what makes both detach-on-switch and reconnect-on-re-entry work; without it, re-entry can't find the live PTY.
- Add `terminalDetach(connectionId)` to `crowbar-bridge.ts`: close **only** the PTY WebSocket (browser `ws.close()` / Tauri `terminal_close`), remove the in-memory transport entry, but **do not** issue `DELETE` and **do not** clear `sessionBases` (kept so re-entry can re-dial `…/terminals/:id/ws`). Keep `terminalClose` (close + `DELETE`) for real tab-close and `exit`.
- In `destroyWorkspaceStore`, for each terminal **look up the daemon `connectionId` first** (via the store, exactly as `killTerminalSession` does today — passing the bufferId would no-op against the connectionId-keyed maps) and call `terminalDetach(connectionId)`. For **bottom-panel** terminals also call `useTerminalStore.removeSession(sessionId)` so `TerminalHost` purges the parked xterm/WebGL canvas (otherwise it leaks and would double-render the ring on re-attach). For **pane** terminals, React already disposes the xterm on unmount, so do **not** drop the daemon mapping — it must survive the switch for reconnect. The daemon PTY stays alive in all cases (no `DELETE`).
- Add `terminalAttach(daemonConnectionId, wsBase)`: open the WS to an **existing** PTY without a `POST`, register the transport entry so `terminalListen`/`Write`/`Resize` stop no-opping (Tauri: reuse `terminal_open`).
- Reconnect on re-entry: discover live sessions via `GET …/workspaces/:w/terminals` (daemon-authoritative), reconcile against the localStorage mapping, set `connectionId` in `useTerminalStore`, and call `terminalAttach(...)`. **Ordering matters:** the `connectionId` must be populated **before `XtermTerminal` first mounts** — if it is set after mount, the create branch runs and `initialCommand` re-executes. Correct the design note: `reuseExistingConnection` only suppresses re-sending the initial command — it does not open or reuse a transport.
- **OSC 7 fix (load-bearing).** `parseOSC7` (`osc-parser.ts`) must use a **global** regex and return the **last** match (today: non-global, first match), so a replay burst sets the *latest* CWD, not the oldest. This alone gives correct reconnect CWD on both browser and Tauri. **`isReplay` frame tagging is dropped from Phase 1** — it would require engine + wire-protocol + Tauri-bridge changes out of Phase 1 scope and is unnecessary once `parseOSC7` returns the last match. (An `isReplay` annotation may be added in a later phase that explicitly scopes the wire change.)

**Phase 2/3:**
- Promote the mapping to the **IndexedDB session index** for cross-restart discovery, keyed by **tab `sessionId`** (the primary key of `workspaceStore.terminals`), storing `{ sessionId, daemonSessionId }`. Gate behind the existing `idb.ts` versioning (pre-production: a version bump clears caches, per the no-legacy-migration rule). On mount, rehydrate `useTerminalStore` from this index ∪ the daemon list.
- Session-state UI: `Live` (no indicator), `Suspended` (grey dot, "will restore on open"), `Dead`/Exited (exit-code indicator). Fed by the extended status frames.

## Error Handling

- **Restore fails (CWD gone):** spawn shell in the workspace root, write a one-line notice into the terminal.
- **Corrupt/missing `.buf`:** start with empty scrollback; metadata row still drives CWD/shell. Log.
- **Torn flush:** atomic temp+rename guarantees the prior good `.buf` survives; never overwrite in place.
- **Ring full:** oldest bytes overwritten silently (expected).
- **Disk full:** log and continue; the in-memory ring is still valid for the live session.

## Testing & Acceptance

**Unit:** ring `Flush`/`Load` round-trip incl. wrap-around (`ring_test.go`); `Session` state transitions + `IsIdle()` with/without a foreground child (`session_test.go`); atomic `.buf` write/read; replay→live handoff under `-race` (snapshot ⧺ live frames == produced stream, no dup/loss).

**Integration** (`api/tests/integration/terminal/`, `integration` tag, as `TestRegression_*` — the v0 contract): detach then re-attach a fresh WS to the same id → scrollback replay + live resume; PTY survives N detach/attach cycles; suspend → restore spawns a shell in the saved CWD with no `ended` frame; daemon restart reloads suspended sessions and a re-attach restores; updated lifecycle DTO/snapshot contract (the three hardcoded `"active"` sites now emit real state — update the existing `m["status"]=="active"` assertions).

**Live Tauri** (per the "manual Tauri in loop" rule — no "works" claim without it): switch workspaces with a running process and confirm it keeps producing output; force-quit + relaunch and confirm scrollback/CWD restore; reboot and confirm restore (process dead, scrollback intact). Verify the replay byte-burst does not cause INP jank.

**Acceptance per phase:**
- *Phase 1:* switch away and back → PTY + scrollback intact; a running foreground process is still alive; no leaked xterm/WS for visited workspaces.
- *Phase 2:* idle shells above the soft limit / global ceiling are suspended and transparently restored on re-entry; a running detached process is never suspended except by the last-resort global ceiling; suspended sessions show the correct UI state.
- *Phase 3:* after app quit/relaunch and after machine reboot, sessions restore (fresh shell, saved CWD, replayed scrollback); graceful shutdown loses no more than the flush interval of scrollback.

## What This Does NOT Solve

- **Running processes across process-tree teardown.** A `npm run dev` survives a workspace switch (daemon alive) but dies on app close/crash/machine restart — no local mechanism can freeze and restore a running macOS process. Scrollback + CWD are restored; the process is not.
- **Live shell state across suspend/restore.** Suspend kills the shell and Restore spawns a fresh one (`ptyEnv()` + profile startup only). Interactively-set env vars/exports, shell functions/aliases, an activated venv (`VIRTUAL_ENV`/`PATH`), the dirstack, and in-shell history position are lost — even though the replayed scrollback makes the terminal *look* unchanged. Only CWD and visual scrollback survive.
- **CWD accuracy when the shell emits no OSC 7.** The primary CWD source is the OSC 7 pump scan. If the shell is configured without OSC 7 emission and the optional pure-Go `proc_info` darwin fallback is not in play (or on Windows/Linux), restore lands in the create-time directory / workspace root. cgo is intentionally not used (the API builds `CGO_ENABLED=0`).
- **Background daemon activity while the app is fully closed.** Deferred with launchd. Until then the daemon runs only while Crowbar is open; restart-restore still works on app open. Windows/Linux have no boot-time daemon (follow-up: Windows service / systemd user unit).

## Superseded Decisions

This design **supersedes decision D6** ("terminal sessions are ephemeral; no `terminal_sessions` view.db"). When implementing Phase 2 (which introduces the `TerminalSession` view.db model), update or remove the D6 references and their assertions: `api/internal/api/v0/dto/terminal.go`, `container.go`, `snapshots.go`, `endpoints/terminal/handlers/sessions.go`, and the contract test `container_terminals_test.go` (`TestTerminalsDef_SnapshotFromEngine`). The lifecycle snapshot now reflects persisted detached/suspended sessions, not only the live registry.

## Implementation Order

1. **Phase 1** — localStorage tab→`connectionId` mapping; frontend `terminalDetach(connectionId)` (look up connectionId first) + `terminalAttach` + reconnect-on-re-entry (populate `connectionId` before mount) + `parseOSC7` global/last-match; remove the kill on switch (keep daemon PTY alive; purge only bottom-panel xterms). Verify live in Tauri.
2. **Phase 2** — DI prerequisite (`TerminalSession` view.db model + `SessionMetaStore` port + storage-path threading); ring `Flush`/`Load` + resize; session lifecycle (derived state, placeholder/`IsLive`, `IsIdle`, `suspending`, `flushMu`, OSC 7 scan, handoff lock, `Kill` lock discipline, flush goroutine); engine restore-aware `Attach` + `Suspend`/`Restore`/auto-suspend/reap-honors-suspending/exit-code; hardened atomic persistence; cadence flush (`.buf` + row); wire-protocol status + snapshot re-source; limits + observability.
3. **Phase 3** — `LoadPersistedSessions` + reconcile-on-open; graceful shutdown flush-then-kill; Tauri SIGTERM-on-close. (launchd deferred.)
