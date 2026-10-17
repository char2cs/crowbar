# Endpoint-security load with no agents running

## Problem

Falcon spikes from ~2% to ~200% CPU while Crowbar is open, with no agent
running. Falcon's cost scales with what it must inspect: process execs, file
opens, and file create/write/rename events. The earlier plans
(`2026-10-15-git-recompute-spawns.md`, `2026-10-15-http-hooks.md`) were
measured with `strace` on Linux and cut spawns per burst and per hook. They
could not see the dominant macOS cost.

## Evidence (macOS, installed app, two workspaces open)

- The daemon held 8,601 descriptors: 7,204 regular files, 1,351 dirs, about
  4,200 per workspace. That is every non-ignored file in each worktree
  (3,545 tracked + 577 dirs + untracked). fsnotify's kqueue backend opens
  every file in each watched directory (`O_EVTONLY`). Each `Start` (workspace
  open, every resubscribe after the 10s linger, every WS reconnect) re-opens
  them, and every directory change re-lists, stats and opens new entries.
  inotify watches directories only, which is why Linux never showed it.
- A recompute costs more than the watcher's 2 spawns. Workspace broadcast runs
  `for-each-ref` and `merge-tree --write-tree` (which writes objects). The
  web's git/status frame then fetches status and log and reloads review files
  (4-6 more spawns).
- Pollers run with no agents:
  - `gh pr list` every minute for the active workspace. (An unchanged result
    already writes nothing: the repository skips a no-op `SyncProviderState`.)
  - `git fetch` every 5 minutes per protected root.
  - Bare `git` through the `/usr/bin/git` xcrun shim (2 execs per call).
  - `binpath.Resolve` is uncached.

## Instrument: the meter (black box, no root)

Falcon itself cannot run here, so we count what it is charged for, against
the real app:

- **Instance:** the `scripts/e2e` isolated instance (own `CROWBAR_HOME`,
  socket and webview origin; tauri target) with a clone of this repo as the
  workspace.
- **Counters:**
  - File opens and execs: a `DYLD_INSERT_LIBRARIES` interposer in the dev
    daemon (unsigned, so dyld honours it; verified on a Go binary) and in the
    children that inherit it.
  - Spawns: counting shims first on `PATH` for git/gh/glab/codex/curl, as a
    cross-check.
  - Held descriptors: `lsof` samples.
  - Files the daemon causes to change: an FSEvents observer over
    `CROWBAR_HOME` and the repo's git dirs.
- **Scenarios:** cold open; 5 min idle; 10 workspace switches; an external
  file burst; a `git fetch` in a sibling worktree.
- **Output:** counts per scenario, before and after each fix.

## Fixes, each starting with a failing behavioural test

1. **FSEvents on macOS.** The watcher gets a backend seam: inotify (fsnotify)
   on Linux, an FSEvents stream on darwin covering the worktree, gitdir and
   common dir. No per-file opens and no walk on start. Darwin daemon builds
   switch to `CGO_ENABLED=1` (release runners are already macOS). Test: held
   descriptors do not grow with the file count.
2. **One git computation per burst.** The daemon's watcher owns git state. The
   frame carries the status it already computed, so the web stops re-fetching.
   Review files reload only while the review surface is visible. Broadcast
   enrichment runs only when branch tips move. Test: N writes produce a fixed
   spawn count end to end.
3. **Pollers do nothing when nothing changed.**
   - Every git exec uses `binpath.Git()`.
   - `binpath.Resolve` caches its answer.
   - Further gating (focus or visibility) is decided from the meter's idle
     numbers.
4. **Agent runs.** Count the `curl` execs left by the HTTP-hooks change
   (status line, Codex hooks) and cut them if they show up.

## Result (`make meter`, fixture of this repo, two workspaces)

| scenario | execs before → after | opens before → after | daemon fds before → after |
| --- | --- | --- | --- |
| cold-open (32s) | 92 → 33 | 17,048 → 241 | 8,593 → 133 |
| idle (310s) | 545 → 4 | 568 → 17 | 8,592 → 132 |
| 10 switches | 72 → 0 | 123 → 12 | 8,592 → 132 |
| burst of 50 changes | 63 → 13 | 205 → 67 | 8,623 → 133 |
| fetch in a sibling | 61 → 17 | 75 → 17 | 8,628 → 133 |

What moved it:
- **FSEvents.** It removed the per-file opens and also a loop that kqueue
  caused: every workspace ran `status` + `diff --numstat` about every 2.4s
  while idle.
- **Provider detection.** It is now keyed by the repo's common dir, merges
  concurrent misses, and parses the slug from the cached remote instead of
  running `git remote get-url` per call.

What remains while idle: one detection per repo per 5 min, plus the 5-min
origin fetch. `gh pr list` is not exercised by the fixture (local remote).

## Acceptance (meter)

- **Idle after settle:** zero execs and zero file opens.
- **App open and workspace switch:** cost proportional to dirs and
  workspaces, not files.
- **Burst:** a fixed number of git spawns.
- Final check: the user opens a build of this branch on the Falcon machine.
