# Endpoint-security meter

Counts what an endpoint-security agent (Falcon and the like) is charged for
while Crowbar runs: process execs, file opens, held descriptors and file
changes, caused by the desktop app, its daemon and their children. No root.
Plan and acceptance targets: `docs/plans/2026-10-16-endpoint-security-load.md`.

    make meter                                  # every scenario, ~10 min
    make meter ARGS=--only=cold-open,burst      # a subset (setup always runs)
    make meter ARGS='--idle=60 --tail=20'       # shorter idle window and tails
    make meter ARGS=--shims                     # also count through PATH shims
    make meter ARGS=--keep                      # keep the fixture repos too

It drives the real desktop app (`tauri dev`, WKWebView, sidecar daemon on a
unix socket) through the `scripts/e2e` instance: own `CROWBAR_HOME`, socket and
webview origin; it never touches `~/.crowbar`. It refuses to start while
another dev `crowbar-api`/`crowbar-desktop` runs. Needs bun, Go, Rust, clang.

## Fixture

A clone of this checkout's HEAD (~3,500 tracked files) imported as a project,
checked out on a side branch so `main` gets its own locked worktree (the
protected root), plus one feature worktree. `origin` is a local bare repo
whose path contains `github.com`, so the daemon detects GitHub and runs its
`gh` checks while fetches stay local. A sibling worktree, not imported, is where the fetch
scenario runs. Each workspace has an open view on a stub-provider chat (`cat`
on a PTY, idle; no real agent).

## Scenarios

Every window is its action followed by a fixed observation tail (`--tail`,
default 30s), so runs compare; the report says when, if ever, the traced
processes first went quiet for 3s.

| name      | action                                                                                   |
| --------- | ---------------------------------------------------------------------------------------- |
| cold-open | relaunch the app and reopen both views                                                   |
| idle      | nothing, for `--idle` seconds (default 310: spans the 1-min PR poll and the 5-min fetch) |
| switch    | 10 workspace switches through Recents                                                    |
| burst     | ~50 creates/edits/deletes over ~5s in the feature worktree                               |
| fetch     | a new commit lands on origin, then `git fetch` in the sibling worktree                   |

Output: a block per scenario and a summary table on stdout, and
`<run dir>/meter/report.json`. The run dir (daemon/app logs, raw
`meter/trace.log`) is kept; the fixture repos are removed unless `--keep`.

## Counters (`native/`, compiled into the run dir at start)

- `interpose.c`: a `DYLD_INSERT_LIBRARIES` interposer over open/openat/creat,
  rename/renameat, unlink/unlinkat, mkdir/mkdirat, execve, posix_spawn(p).
  cargo's runner sets it on the app (DYLD_* would not survive `bunx`, which
  starts the Tauri CLI through SIP's `/usr/bin/env`); the sidecar and every
  child inherit it, re-injected into each child's environment.
- `shim.c` (`--shims`): counting shims for git, gh, glab, codex, curl, first
  on PATH. The daemon puts its login shell's PATH first, so the meter also
  wraps `$SHELL` for that one probe. Off by default: the extra hop changes what
  is measured (every git goes through the shim instead of the direct path).
- `lsof` samples of the daemon's descriptors at every window boundary.
- `fsevents.c`: an FSEvents stream over the home, the project dir and the
  sibling worktree; sees writes by processes the interposer cannot enter.

Positive controls run every time and fail the run when a counter saw nothing:
the interposer's `load` line from the daemon's own pid with opens logged, a
canary file seen by FSEvents, a non-empty lsof sample, shim calls (`--shims`).

## Limits

- SIP binaries (`/usr/bin/*`, `/bin/*`) and library-validated ones (Xcode's
  git) never load the interposer: their execs are counted from the parent,
  but their own opens, writes and children are invisible (git's internal
  execs, and xcrun's second exec behind `/usr/bin/git`). FSEvents covers
  their file changes under the watched roots, without attributing them.
- WKWebView's WebContent/Networking processes are started by launchd (XPC)
  and are not counted.
- gh cannot map the local remote to a GitHub host, and the fixture's branches
  have no PRs: `gh auth status` runs, PR lookups are not exercised.
- Opens are syscalls, not Falcon events: reads of the same file count once
  per open. fsnotify's kqueue opens (`O_EVTONLY`) are included.
