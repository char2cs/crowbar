// The materialisation planner for the windowed Branch Review renderer.
//
// The review surface reserves scroll space for every changed file from the
// outline's hunk geometry, but only ever holds the PATCH TEXT of the handful of
// files near the viewport. planWindow is the decision of which handful: given
// where the viewport is and what is currently in memory, it answers which
// patches to fetch and which to drop.
//
// It is deliberately pure — no React, no fetch, no timers — because this is the
// one place the phase is either correct or thrashing, and purity is what lets it
// be tested exhaustively without a DOM, a network or a virtualiser.

/** How many files past each edge of the viewport are materialised ahead of time. */
export const LOOKAHEAD_FILES = 6

/**
 * How far a materialised file may drift from the viewport before it is dropped.
 *
 * This band is deliberately WIDER than LOOKAHEAD_FILES, and the gap between them
 * is the whole anti-thrash mechanism. Were the two equal, a file sitting exactly
 * at the boundary would be evicted by a one-row scroll and refetched by the
 * scroll back — a network request per frame, forever. The gap gives a file that
 * has left the fetch band somewhere to wait.
 */
export const EVICT_BEYOND_FILES = 20

/** Hard ceiling on how many files hold patch text at once. */
export const MAX_MATERIALIZED_FILES = 40

/**
 * Hard ceiling on the total diff lines held at once.
 *
 * The file cap alone does not bound memory: forty ordinary files are nothing,
 * while two generated ones are a million lines. This is the cap that actually
 * holds the renderer flat across a branch like the 1M-line fixture.
 */
export const MAX_MATERIALIZED_LINES = 60_000

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

/**
 * Decide which patches this frame should hold.
 *
 * The plan is a pure function of the input, so applying it and re-planning at
 * the same scroll position yields an empty plan — the property that makes
 * scrolling converge rather than oscillate. Three rules produce it:
 *
 *  1. Everything within LOOKAHEAD_FILES of the viewport is fetched.
 *  2. Anything beyond EVICT_BEYOND_FILES is dropped; between the two bands a
 *     file is simply left alone (see EVICT_BEYOND_FILES).
 *  3. What survives is trimmed to the budgets, furthest-from-viewport first —
 *     and the trim is applied to the files this frame WOULD fetch as well as to
 *     the ones it already holds, so the budget can never be blown by a fetch and
 *     then immediately recovered by evicting it back.
 *
 * A VISIBLE file is exempt from rule 3 in both directions: it is on screen, and
 * showing the user a blank pane to stay under a memory budget is not a trade
 * worth making. A single 500,000-line file therefore renders, alone.
 */
export function planWindow(input: WindowInput): WindowPlan {
  const count = Math.min(input.total, input.paths.length)
  if (count <= 0) return { ...EMPTY_PLAN }

  const indexOf = buildIndex(input.paths, count)
  const { first, last } = clampViewport(input.visible, count)
  // `materialized` is a set by meaning; treating it as one keeps a caller that
  // repeated a path from producing that path twice in the plan.
  const held = new Set(input.materialized)

  // A materialised path that is no longer in the file list describes a diff that
  // no longer exists; it can only be dropped.
  const evict = [...held].filter((p) => !indexOf.has(p))

  const candidates: Candidate[] = []
  for (const path of held) {
    const index = indexOf.get(path)
    if (index === undefined) continue
    const distance = distanceFrom(index, first, last)
    if (distance > EVICT_BEYOND_FILES) {
      evict.push(path)
      continue
    }
    candidates.push({ path, index, distance, held: true })
  }
  for (let index = bandStart(first); index <= bandEnd(last, count); index++) {
    const path = input.paths[index]
    if (held.has(path)) continue
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

/**
 * Trim the frame's candidates down to MAX_MATERIALIZED_FILES and
 * MAX_MATERIALIZED_LINES, returning what had to go.
 *
 * Trimming stops when the only candidates left are visible ones, even if the
 * budgets are still exceeded — the budgets bound what is held OFF screen, and
 * the user is looking at the rest.
 */
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
  return lineCounts[c.path] ?? 0
}

/** Nearest the viewport first, so the consumer's first request is the one on screen. */
function byNearestFirst(a: Candidate, b: Candidate): number {
  return a.distance - b.distance || a.index - b.index
}

/**
 * Furthest from the viewport first. At equal distance the file ABOVE the
 * viewport goes first: scrolling forward is the common direction, so the file
 * the reader has already passed is the one less likely to be wanted back.
 */
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
