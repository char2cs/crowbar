const MINUTE_MS = 60_000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS

/** "Aug 30, 6:36 PM" — the year only when it isn't the current one, since
 *  spelling it out on every turn in an active chat is noise nobody reads. */
function absoluteLabel(then: number): string {
  const date = new Date(then)
  const sameYear = date.getFullYear() === new Date().getFullYear()
  const datePart = date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: sameYear ? undefined : 'numeric',
  })
  const timePart = date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `${datePart}, ${timePart}`
}

/** A raw duration, rounded to whichever single unit it actually reads as:
 *  minutes under an hour, hours under a day, days beyond that. Floored, not
 *  rounded to nearest — "1h" claiming a FULL hour has passed when only 85
 *  minutes have overclaims exactly as much as "127m" undersells it; the
 *  point is never to imply more precision than a reader needs, not to hide
 *  precision they'd want. */
function durationLabel(elapsedMs: number): string {
  const elapsed = Math.max(0, elapsedMs)
  if (elapsed < HOUR_MS) return `${Math.floor(elapsed / MINUTE_MS)}m`
  if (elapsed < DAY_MS) return `${Math.floor(elapsed / HOUR_MS)}h`
  return `${Math.floor(elapsed / DAY_MS)}d`
}

/** A user prompt's own timestamp — absolute only. There is nothing to time
 *  IT against; "elapsed" only ever means something for the reply that
 *  answers it. */
export function turnTimestampLabel(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return absoluteLabel(then)
}

/** How long the agent took to answer — the gap between the user's own turn
 *  and this reply, not "how long ago", paired with the absolute time the
 *  reply landed: "3m · Aug 30, 6:36 PM", rounding to hours or days once the
 *  gap is that large. A reply is, definitionally, never older than the
 *  prompt it answers, so this never needs a now-vs-then fallback the way
 *  `turnTimeLabel` does. */
export function turnLatencyLabel(userAt: string, assistantAt: string): string {
  const start = Date.parse(userAt)
  const end = Date.parse(assistantAt)
  if (Number.isNaN(start) || Number.isNaN(end)) return ''
  return `${durationLabel(end - start)} · ${absoluteLabel(end)}`
}

/** How long ago a turn happened, AND when — both visible, not one hidden
 *  behind a hover, rounding to hours or days once that long has actually
 *  passed. FALLBACK ONLY, for an assistant message with no preceding user
 *  turn to time itself against (e.g. a harness-injected reply) — every
 *  ordinary reply uses `turnLatencyLabel` instead. */
export function turnTimeLabel(at: string, now: number = Date.now()): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return `${durationLabel(now - then)} · ${absoluteLabel(then)}`
}

/** The full, unambiguous timestamp — on hover, alongside whichever shorthand
 *  is showing inline. */
export function turnTimeTitle(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return new Date(then).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}
