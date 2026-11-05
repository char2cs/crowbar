// Decides which patches the windowed Branch Review holds: given the viewport and what is in
// memory, which to fetch and which to drop. Pure (no React, fetch or timers) so the one place
// that is either correct or thrashing can be tested exhaustively.

/** How many files past each edge of the viewport are materialised ahead of time. */
export const LOOKAHEAD_FILES = 6

/** Drift allowed before a held file drops. Wider than LOOKAHEAD_FILES on purpose: equal bands
 *  would evict and refetch a boundary file on every one-row scroll. */
export const EVICT_BEYOND_FILES = 20

/** Hard ceiling on how many files hold patch text at once. */
export const MAX_MATERIALIZED_FILES = 40

/** Hard ceiling on total diff lines held — what actually bounds memory, since two generated
 *  files can outweigh forty ordinary ones. */
export const MAX_MATERIALIZED_LINES = 60_000

/** The server's default per-file patch cap. Budgets count this, not the ± count: a capped fetch
 *  is all a file costs, and budgeting the full diff would never fetch a huge file at all. */
export const PATCH_LINE_CAP = 20_000

export interface WindowInput {
  /** Inclusive index range of the files the virtualiser currently has on screen. */
  visible: { first: number; last: number }
  /** The virtualiser's item count. Authoritative: the band never runs past it. */
  total: number
  /** Paths whose patch text is held right now. */
  materialized: readonly string[]
  /** Every changed file, in render order. Indices in `visible` address this list. */
  paths: readonly string[]
  /** Per-path diff line count, from the outline geometry — known before any fetch. */
  lineCounts: Readonly<Record<string, number>>
  /** Viewed paths render as a bare header: never fetched, dropped if held. */
  viewed?: ReadonlySet<string>
}

export interface WindowPlan {
  /** Patches to fetch, nearest-to-viewport first. */
  fetch: string[]
  /** Patches to drop, furthest-from-viewport first. */
  evict: string[]
}

/** A file considered for this frame, with its distance from the viewport. */
interface Candidate {
  path: string
  index: number
  /** Rows between the file and the nearest viewport edge; 0 means on screen. */
  distance: number
  /** True when the file already holds patch text (so dropping it is an eviction). */
  held: boolean
}

const EMPTY_PLAN: WindowPlan = { fetch: [], evict: [] }

// Re-planning after applying a plan yields an empty one, so scrolling converges. Fetch within
// LOOKAHEAD, drop past EVICT_BEYOND, then trim held and to-fetch files alike to budget, furthest
// first — never a visible file, so even one huge file renders.
export function planWindow(input: WindowInput): WindowPlan {
  const count = Math.min(input.total, input.paths.length)
  if (count <= 0) return { ...EMPTY_PLAN }

  const indexOf = buildIndex(input.paths, count)
  const { first, last } = clampViewport(input.visible, count)
  // `materialized` is a set by meaning; treating it as one keeps a caller that
  // repeated a path from producing that path twice in the plan.
  const held = new Set(input.materialized)

  // A held path missing from the file list describes a diff that is gone; it can only drop.
  const evict = [...held].filter((p) => !indexOf.has(p))
  const viewed = input.viewed

  const candidates: Candidate[] = []
  for (const path of held) {
    const index = indexOf.get(path)
    if (index === undefined) continue
    if (viewed?.has(path)) {
      evict.push(path)
      continue
    }
    const distance = distanceFrom(index, first, last)
    if (distance > EVICT_BEYOND_FILES) {
      evict.push(path)
      continue
    }
    candidates.push({ path, index, distance, held: true })
  }
  for (let index = bandStart(first); index <= bandEnd(last, count); index++) {
    const path = input.paths[index]
    if (held.has(path) || viewed?.has(path)) continue
    candidates.push({ path, index, distance: distanceFrom(index, first, last), held: false })
  }

  const dropped = new Set(applyBudgets(candidates, input.lineCounts))
  for (const c of dropped) {
    if (c.held) evict.push(c.path)
  }

  const fetch = candidates.filter((c) => !c.held && !dropped.has(c))
  return {
    fetch: fetch.sort(byNearestFirst).map((c) => c.path),
    evict: sortEvictions(evict, indexOf, first, last),
  }
}

/** First index of each path, capped at the authoritative item count. */
function buildIndex(paths: readonly string[], count: number): Map<string, number> {
  const indexOf = new Map<string, number>()
  for (let i = 0; i < count; i++) {
    if (!indexOf.has(paths[i])) indexOf.set(paths[i], i)
  }
  return indexOf
}

/** A viewport that ran past the ends of the list still has to describe real rows. */
function clampViewport(
  visible: { first: number; last: number },
  count: number,
): { first: number; last: number } {
  const first = clamp(visible.first, 0, count - 1)
  return { first, last: Math.max(first, clamp(visible.last, 0, count - 1)) }
}

function clamp(n: number, lo: number, hi: number): number {
  if (!Number.isFinite(n)) return lo
  return Math.min(hi, Math.max(lo, Math.trunc(n)))
}

function bandStart(first: number): number {
  return Math.max(0, first - LOOKAHEAD_FILES)
}

function bandEnd(last: number, count: number): number {
  return Math.min(count - 1, last + LOOKAHEAD_FILES)
}

function distanceFrom(index: number, first: number, last: number): number {
  if (index < first) return first - index
  if (index > last) return index - last
  return 0
}

/** Trims candidates to the file and line budgets, returning what went. Visible ones are never
 *  trimmed: the budgets bound what is held off screen. */
function applyBudgets(
  candidates: readonly Candidate[],
  lineCounts: Readonly<Record<string, number>>,
): Candidate[] {
  const order = [...candidates].sort(byFurthestFirst)
  let files = candidates.length
  let lines = candidates.reduce((sum, c) => sum + linesOf(c, lineCounts), 0)

  const dropped: Candidate[] = []
  for (const c of order) {
    if (files <= MAX_MATERIALIZED_FILES && lines <= MAX_MATERIALIZED_LINES) break
    if (c.distance === 0) continue // on screen — never traded away for a budget
    dropped.push(c)
    files--
    lines -= linesOf(c, lineCounts)
  }
  return dropped
}

function linesOf(c: Candidate, lineCounts: Readonly<Record<string, number>>): number {
  // Capped: the fetch is capped, so this is what will be held. See PATCH_LINE_CAP.
  return Math.min(lineCounts[c.path] ?? 0, PATCH_LINE_CAP)
}

/** Nearest the viewport first, so the consumer's first request is the one on screen. */
function byNearestFirst(a: Candidate, b: Candidate): number {
  return a.distance - b.distance || a.index - b.index
}

/** Furthest first; at equal distance the file above goes first, since readers mostly scroll
 *  forward. */
function byFurthestFirst(a: Candidate, b: Candidate): number {
  return b.distance - a.distance || a.index - b.index
}

/** Order evictions furthest-first; a path that has left the file list goes first of all. */
function sortEvictions(
  paths: readonly string[],
  indexOf: ReadonlyMap<string, number>,
  first: number,
  last: number,
): string[] {
  return [...paths].sort((a, b) => {
    const ia = indexOf.get(a)
    const ib = indexOf.get(b)
    const da = ia === undefined ? Infinity : distanceFrom(ia, first, last)
    const db = ib === undefined ? Infinity : distanceFrom(ib, first, last)
    if (da !== db) return db - da
    return (ia ?? 0) - (ib ?? 0)
  })
}
