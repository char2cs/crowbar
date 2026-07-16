/**
 * Warm-reactivation freshness ledger (spec §4 / Task 33 Target A).
 *
 * WorkspaceHost keeps recently-visited workspaces mounted but hidden; only the
 * ACTIVE workspace's watchers (use-workspace-effects) seed the GLOBAL file-tree
 * and git stores. On every hidden→active flip those watchers remount and
 * re-seed from scratch — a full file-tree fetch plus a four-request git load —
 * even when the user only glanced away for a moment and nothing changed. That
 * re-seed is the bulk of the warm-switch cost measured in Task 32 (the fan-out
 * plus the synchronous `isFileTreeLoading:true` re-render on the switch frame).
 *
 * This ledger lets a warm reactivation SKIP that re-seed when it is provably
 * safe. It is a pure optimisation — correctness-first — so the Task 29
 * stale-tree bug cannot return:
 *
 *   - The window bounds the risk from OUT-OF-BAND edits (a terminal or agent
 *     writing a hidden worktree — undetectable while its file watcher is torn
 *     down). Past the window we always re-seed; inside it we keep the data.
 *   - Callers pair this with a STORE-IDENTITY guard: the fast path is refused
 *     whenever another workspace clobbered the global store while we were away
 *     (`rootFolderPath` / `currentWorkspaceRepoPath` no longer match this ws).
 *   - Git status/log self-heal regardless of the window: the fast path
 *     re-subscribes the git/status stream seeded with the PRESERVED last frame,
 *     so a frame that differs (something changed while hidden) still triggers
 *     the normal reload. Branches/stashes change only on explicit git actions,
 *     so keeping the loaded values is correct.
 *
 * Self-heal ASYMMETRY — why the window is load-bearing for the TREE but only a
 * heuristic for git: the git/status stream pushes a snapshot frame on every
 * (re)subscribe, so a change that landed while hidden is caught on reattach no
 * matter how long the gap was. The files stream has NO snapshot-on-subscribe —
 * it emits only future change events — so a structural change that happened
 * while the watcher was torn down is invisible to a kept tree. Past the window
 * the tree MUST re-seed unconditionally; there is no stream to catch it up.
 */

// A workspace hidden no longer than this may keep its already-loaded data on a
// warm return. Short by design: it only has to cover the "glanced away" cases
// (rapid A↔B switching, an A→home→A transit) that keep-alive exists for, while
// staying well under the horizon where an out-of-band edit becomes likely.
// Anything hidden longer re-seeds — cheap insurance against a stale tree.
export const WARM_FRESHNESS_WINDOW_MS = 5_000

// wsId → time it last went hidden (active → inactive). Read on the next warm
// return to measure how long it was away.
const deactivatedAt = new Map<string, number>()

// wsId → the last git/status frame seen while active, preserved across the
// hidden gap so a re-subscribe that pushes the SAME frame doesn't retrigger a
// reload (a differing frame still does — that is the self-heal).
const gitFrames = new Map<string, unknown>()

/** Record that `wsId` just went hidden (active → inactive). */
export function markWorkspaceDeactivated(wsId: string, now: number = Date.now()): void {
  deactivatedAt.set(wsId, now)
}

/**
 * True when `wsId` was hidden for no longer than the freshness window — i.e. a
 * warm return may keep its already-loaded data instead of re-seeding. Does NOT
 * consume the stamp: the file-tree and git seed effects both consult it on the
 * same activation and must agree. Callers MUST additionally confirm the global
 * store still holds this workspace's data before taking the fast path.
 */
export function isWarmDataFresh(wsId: string, now: number = Date.now()): boolean {
  const at = deactivatedAt.get(wsId)
  return at !== undefined && now - at <= WARM_FRESHNESS_WINDOW_MS
}

/** Preserve the last git/status frame across a hidden gap (dedupes the re-push). */
export function saveGitFrame(wsId: string, frame: unknown): void {
  gitFrames.set(wsId, frame)
}

/** Peek at the preserved git/status frame (null if none) to seed a re-subscribe. */
export function peekGitFrame(wsId: string): unknown {
  return gitFrames.has(wsId) ? gitFrames.get(wsId) : null
}

/** Drop all ledger state for a destroyed / evicted workspace. */
export function clearWorkspaceFreshness(wsId: string): void {
  deactivatedAt.delete(wsId)
  gitFrames.delete(wsId)
}

/** Test-only: wipe the whole ledger between cases. */
export function __resetActivationFreshnessForTests(): void {
  deactivatedAt.clear()
  gitFrames.clear()
}
