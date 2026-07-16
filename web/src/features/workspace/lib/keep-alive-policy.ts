/**
 * Pure retention policy for workspace keep-alive (spec §4 / P3).
 *
 * Given the mounted workspaces and their last-active timestamps, decides which
 * to keep in memory and which to destroy, plus when the next timer-driven
 * eviction should run. No `Date.now()` here — `now` is injected so the function
 * stays deterministic and unit-testable; the host owns the clock.
 *
 * Invariants:
 *  - The most-recently-active workspace is ALWAYS retained. The host refreshes
 *    the active workspace's timestamp to `now` before every call, so "most
 *    recent" is the active workspace — this is what guarantees "the active
 *    workspace is never evicted, even on the timer path when it has sat idle
 *    past the window".
 *  - A non-active workspace is retained while its age (`now - lastActiveAt`) is
 *    strictly less than `windowMs`. The boundary (age === windowMs) counts as
 *    expired so a timer armed for exactly that instant evicts it instead of
 *    re-arming for the same instant forever.
 *  - At most `cap` workspaces are retained (xterm WebGL context ceiling). When
 *    more than `cap` are within the window, the oldest are evicted regardless.
 *  - `windowMs === 0` reproduces destroy-on-switch: only the active workspace
 *    survives.
 */

/** Hard cap on simultaneously-retained workspaces (xterm WebGL contexts). */
export const RETENTION_CAP = 6

export interface RetentionEntry {
  wsId: string
  lastActiveAt: number
}

export interface RetentionPlan {
  /** Workspace ids to keep mounted, in the input's order. */
  retain: string[]
  /** Workspace ids to destroy now, in the input's order. */
  evict: string[]
  /**
   * Epoch ms at which the next timer-driven eviction should run, or null when
   * nothing is on a timer (no non-active retained entries, or windowMs === 0).
   */
  nextExpiryAt: number | null
}

export function planRetention(
  entries: RetentionEntry[],
  now: number,
  windowMs: number,
  cap: number = RETENTION_CAP,
): RetentionPlan {
  if (entries.length === 0) {
    return { retain: [], evict: [], nextExpiryAt: null }
  }

  // Index the input so we can retain deterministically (recency, then input
  // order for ties) while emitting output in the original input order.
  const indexed = entries.map((entry, index) => ({ ...entry, index }))

  // The most-recently-active entry is the protected (active) workspace.
  const mostRecent = indexed.reduce((best, cur) =>
    cur.lastActiveAt > best.lastActiveAt ? cur : best,
  )

  const hasWindow = windowMs > 0
  const isLive = (entry: (typeof indexed)[number]): boolean =>
    entry.index === mostRecent.index || (hasWindow && now - entry.lastActiveAt < windowMs)

  // Candidate retained set: the active workspace plus every in-window entry.
  const candidates = indexed.filter(isLive)

  // Enforce the hard cap by recency (most-recent first; input order breaks
  // ties). The active workspace has the max timestamp so it always survives.
  const capped = [...candidates]
    .sort((a, b) => b.lastActiveAt - a.lastActiveAt || a.index - b.index)
    .slice(0, Math.max(1, cap))

  const retainIndexes = new Set(capped.map((e) => e.index))

  const retain: string[] = []
  const evict: string[] = []
  for (const entry of indexed) {
    if (retainIndexes.has(entry.index)) retain.push(entry.wsId)
    else evict.push(entry.wsId)
  }

  // The next eviction fires when the earliest-expiring RETAINED non-active
  // entry crosses the window. The active workspace never expires.
  let nextExpiryAt: number | null = null
  if (hasWindow) {
    for (const entry of capped) {
      if (entry.index === mostRecent.index) continue
      const expiry = entry.lastActiveAt + windowMs
      if (nextExpiryAt === null || expiry < nextExpiryAt) nextExpiryAt = expiry
    }
  }

  return { retain, evict, nextExpiryAt }
}
