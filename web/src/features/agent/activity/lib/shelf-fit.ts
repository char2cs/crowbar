export interface ShelfToken {
  id: string
  agentType?: string
  /** Seconds since it started. */
  elapsed: number
}

export interface ShelfLayout {
  shown: ShelfToken[]
  overflow: number
  /** Drop the type names — over four in flight, the count matters more. */
  dense: boolean
}

export const DENSE_THRESHOLD = 4

/** Room reserved for the `+N` counter, in px. */
export const OVERFLOW_RESERVE = 30

/** Sheds names first, then whole tokens into a `+N`, never wrapping. Count and clocks never drop:
 *  they are all `AgentSubagent` carries. */
export function fitShelf(
  tokens: ShelfToken[],
  availableWidth: number,
  measure: (token: ShelfToken, dense: boolean) => number,
): ShelfLayout {
  const dense = tokens.length > DENSE_THRESHOLD
  if (tokens.length === 0) return { shown: [], overflow: 0, dense }

  // An unmeasured row (width 0 before layout) shows everything rather than
  // collapsing to `+N` for a frame and then expanding.
  if (availableWidth <= 0) return { shown: tokens, overflow: 0, dense }

  const budget = availableWidth - OVERFLOW_RESERVE
  let used = 0
  let shown = 0
  for (const token of tokens) {
    const width = measure(token, dense) + 4
    if (used + width > budget) break
    used += width
    shown++
  }
  // One token plus a counter says more than a bare "+N".
  if (shown === 0) shown = 1
  return {
    shown: tokens.slice(0, shown),
    overflow: tokens.length - shown,
    dense,
  }
}

/** m:ss below an hour, then h:mm:ss and d hh:mm:ss — turns run for hours and
 *  `142:07` stops reading as a clock. */
export function formatElapsed(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const pad = (n: number) => String(n).padStart(2, '0')

  const s = safe % 60
  const totalMinutes = Math.floor(safe / 60)
  const m = totalMinutes % 60
  const totalHours = Math.floor(totalMinutes / 60)
  const h = totalHours % 24
  const d = Math.floor(totalHours / 24)

  if (d > 0) return `${d}d ${pad(h)}:${pad(m)}:${pad(s)}`
  if (totalHours > 0) return `${totalHours}:${pad(m)}:${pad(s)}`
  return `${m}:${pad(s)}`
}
