/** A subagent as the shelf needs it: what Crowbar knows, and nothing more. */
export interface ShelfToken {
  id: string
  /** The provider's word for it, when it gave one. */
  agentType?: string
  /** Seconds since it started. */
  elapsed: number
}

export interface ShelfLayout {
  /** Tokens to draw, in order. */
  shown: ShelfToken[]
  /** How many did not fit. 0 draws no counter. */
  overflow: number
  /** Drop the type names — over four in flight, the count matters more. */
  dense: boolean
}

/** Names are dropped at five in flight. Below that they fit and they help. */
export const DENSE_THRESHOLD = 4

/** Room reserved for the `+N` counter, in px. */
export const OVERFLOW_RESERVE = 30

/**
 * How the shelf sheds detail as a fan-out widens.
 *
 * IN A FIXED ORDER, never by wrapping or scrolling: names first, then whole
 * tokens into a `+N`. The COUNT and the CLOCKS never drop, because count and
 * elapsed time are the only two things `AgentSubagent` actually carries — a
 * shelf that hid them to fit would be hiding the whole payload.
 *
 * `measure` returns a token's rendered width; the caller supplies it from the
 * DOM so this stays pure and testable.
 */
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
  // Never render a bare "+N" with nothing beside it: one token plus a counter
  // says more than a counter alone.
  if (shown === 0) shown = 1
  return {
    shown: tokens.slice(0, shown),
    overflow: tokens.length - shown,
    dense,
  }
}

/**
 * m:ss like a stopwatch below an hour, then h:mm:ss and d hh:mm:ss beyond it —
 * subagents and turns routinely run for hours, and minutes alone stop reading
 * as a clock once they climb past 59 (`142:07` instead of `2:22:07`).
 */
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
