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

## Decision (approved, implemented)

1. **Watcher-owned resolved base.** `Watcher.diffBase` holds the engine's
   `ResolveDiffBase` answer until a ref event (`handleOne`, for main and linked
   worktrees alike) clears it. A file-only burst runs `status` and
   `DiffStat` only.
2. **Conflicts from status.** `hasConflicts` is read off the `status` the
   watcher already ran; the `diff --diff-filter=U` spawn is gone from the
   watcher path (`WorkingTreeSummary` for other callers still checks it).
3. **Wait for the burst to end.** The existing trailing debounce goes from 250ms
   to a 2s quiet period (`gitRecomputeQuiet`), bounded by a 10s ceiling
   (`gitRecomputeMaxWait`) so a burst that never ends still updates the UI. This
   is the same debounce with a longer wait and a ceiling, not a new timer. The
   cost is that the git panel lags the last write by up to 2s.

Because the cache is only as good as ref-event delivery, the watcher now
watches the git dir and common dir as directories (git replaces `HEAD` and
`packed-refs` by rename, which ends a watch on the file after the first change)
and every directory of the ref trees, including ones created later (nested
branches, `refs/remotes/<remote>/`). Before this, a fetch or a second `HEAD`
change was only noticed because the next file burst recomputed everything.
Events that cannot change a recompute's answer (index, lock files) no longer
schedule one.

## Result

Seeded workspace, one subscriber, file writes every 400ms for 20s
(`strace -f -e execve`): 294 spawns before, 101 after the base cache alone. Steady state is 2 spawns
per recompute; a commit costs one extra base resolution (`merge-base` plus two
`rev-list`).
