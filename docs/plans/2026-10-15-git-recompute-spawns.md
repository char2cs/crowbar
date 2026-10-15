# Git recompute spawns per file-change burst

## Problem

Falcon (endpoint security) spikes while Crowbar is open. `strace -f` on the
daemon with one subscribed workspace and file writes every 400ms showed ~14
`git` spawns/s and ~550 file opens/s. Idle, and a subscribed-but-quiet
workspace, spawn nothing.

Every burst of working-tree changes arms `Watcher.scheduleGitRecompute`
(250ms trailing debounce), and `fanOutGit` then runs, per workspace:

| spawn | depends on |
| --- | --- |
| `status --porcelain=v2 --branch` | working tree |
| `diff --numstat <base>` | working tree + base |
| `diff --name-only --diff-filter=U` | index |
| `rev-list --count <ref>..HEAD` x2 (origin/b, b) | refs |
| `merge-base <ref> HEAD` | refs |

Measured per recompute: 7 before, 6 after the summary change in this PR
(`resolveDiffBase` now ranks candidates by `rev-list --count ref..HEAD` and
reuses that count for `hasCommits`; one `merge-base` for the winner). With a
real `origin` remote it is 8 -> 6.

## Root cause

The base (merge-base of `origin/<b>`/`<b>` with HEAD, and its commit count) is a
function of refs only, but it is re-derived on every working-tree burst. The
watcher already distinguishes ref events (`maybeHandleGitRef`) from file
events; the summary path does not.

## Decision needed (CLAUDE.md: a second copy of state stops and plans)

Skipping the ref-derived spawns on file-only bursts means holding the resolved
base somewhere between recomputes, invalidated by ref events. That is derived
state beside git's own, so it is written down here rather than shipped.

1. **Watcher-owned resolved base.** `Watcher` keeps the resolved merge-base
   SHA, cleared in `maybeHandleGitRef` (HEAD, refs/, packed-refs are already
   watched via `gitRefWatchPaths`, including a linked worktree's common dir and
   origin fetches). Recompute then passes the SHA as `base`; the engine's SHA
   path skips `origin/<sha>` probing. File-only burst: status + numstat +
   conflicts + one `rev-list` (3-4 spawns). Needs a new engine call to resolve
   the base once, so `git.Engine`, its mocks and `watch.GitStatusProvider`
   change.
2. **Conflicts from status.** `status` already reports unmerged (`u`) entries,
   so `hasConflicts` needs no `diff --diff-filter=U`. The watcher has the
   status when it computes the summary. Needs `GitStatusProvider` to take it.
3. **Do not add a minimum recompute interval.** It is a timer over a symptom
   (CLAUDE.md), and it would delay the status the UI shows.

Together 1 + 2 take a file-only burst from 6-8 spawns to 3 (`status`,
`diff --numstat`, one `rev-list`), and 2 when the sha base is trusted.

## Steps

1. Failing test: N file-only bursts after the first cost zero ref-derived
   spawns; a ref event costs them once.
2. Move base resolution to the watcher; pass SHA to the summary.
3. Derive conflicts from the status the watcher already holds.
4. Update `watcher.go`'s "two git invocations" comment to match.
