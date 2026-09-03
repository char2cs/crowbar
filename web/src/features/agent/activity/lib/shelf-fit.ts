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

/** m:ss, the way a stopwatch reads. Subagents routinely run past a minute. */
export function formatElapsed(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  return `${Math.floor(safe / 60)}:${String(safe % 60).padStart(2, '0')}`
}
