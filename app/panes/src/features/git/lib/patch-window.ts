const LOOKAHEAD_FILES = 6

const EVICT_BEYOND_FILES = 20

const MAX_MATERIALIZED_FILES = 40

export const MAX_MATERIALIZED_LINES = 60_000

export const PATCH_LINE_CAP = 20_000

export interface WindowInput {
  visible: { first: number; last: number }
  total: number
  materialized: readonly string[]
  paths: readonly string[]
  lineCounts: Readonly<Record<string, number>>
  viewed?: ReadonlySet<string>
}

export interface WindowPlan {
  fetch: string[]
  evict: string[]
}

interface Candidate {
  path: string
  index: number
  distance: number
  held: boolean
}

const EMPTY_PLAN: WindowPlan = { fetch: [], evict: [] }

export function planWindow(input: WindowInput): WindowPlan {
  const count = Math.min(input.total, input.paths.length)
  if (count <= 0) return { ...EMPTY_PLAN }

  const indexOf = buildIndex(input.paths, count)
  const { first, last } = clampViewport(input.visible, count)
  const held = new Set(input.materialized)

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

function buildIndex(paths: readonly string[], count: number): Map<string, number> {
  const indexOf = new Map<string, number>()
  for (let i = 0; i < count; i++) {
    if (!indexOf.has(paths[i])) indexOf.set(paths[i], i)
  }
  return indexOf
}

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
    if (c.distance === 0) continue
    dropped.push(c)
    files--
    lines -= linesOf(c, lineCounts)
  }
  return dropped
}

function linesOf(c: Candidate, lineCounts: Readonly<Record<string, number>>): number {
  return Math.min(lineCounts[c.path] ?? 0, PATCH_LINE_CAP)
}

function byNearestFirst(a: Candidate, b: Candidate): number {
  return a.distance - b.distance || a.index - b.index
}

function byFurthestFirst(a: Candidate, b: Candidate): number {
  return b.distance - a.distance || a.index - b.index
}

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
