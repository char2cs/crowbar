const MINUTE_MS = 60_000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS

/** Built once on first use: a formatter resolves the locale when constructed, and
 *  `toLocale*String` builds a fresh one per call — once per row on every mount. */
function memoFormat(options: Intl.DateTimeFormatOptions): (date: Date) => string {
  let formatter: Intl.DateTimeFormat | undefined
  return (date) => (formatter ??= new Intl.DateTimeFormat(undefined, options)).format(date)
}

const dateThisYear = memoFormat({ month: 'short', day: 'numeric' })
const dateOtherYear = memoFormat({ month: 'short', day: 'numeric', year: 'numeric' })
const timeOfDay = memoFormat({ hour: 'numeric', minute: '2-digit' })
const fullTitle = memoFormat({ dateStyle: 'medium', timeStyle: 'short' })

/** "Aug 30, 6:36 PM" — the year only when it isn't the current one. */
function absoluteLabel(then: number): string {
  const date = new Date(then)
  const sameYear = date.getFullYear() === new Date().getFullYear()
  return `${(sameYear ? dateThisYear : dateOtherYear)(date)}, ${timeOfDay(date)}`
}

/** Floored, never rounded up, so "1h" never claims more time than has passed. */
function durationLabel(elapsedMs: number): string {
  const elapsed = Math.max(0, elapsedMs)
  if (elapsed < HOUR_MS) return `${Math.floor(elapsed / MINUTE_MS)}m`
  if (elapsed < DAY_MS) return `${Math.floor(elapsed / HOUR_MS)}h`
  return `${Math.floor(elapsed / DAY_MS)}d`
}

/** Absolute only: there is nothing to time a prompt against. */
export function turnTimestampLabel(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return absoluteLabel(then)
}

/** The gap from prompt to reply, not "how long ago": "3m · Aug 30, 6:36 PM". */
export function turnLatencyLabel(userAt: string, assistantAt: string): string {
  const start = Date.parse(userAt)
  const end = Date.parse(assistantAt)
  if (Number.isNaN(start) || Number.isNaN(end)) return ''
  return `${durationLabel(end - start)} · ${absoluteLabel(end)}`
}

/** Fallback for a reply with no preceding user turn to time itself against. */
export function turnTimeLabel(at: string, now: number = Date.now()): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return `${durationLabel(now - then)} · ${absoluteLabel(then)}`
}

export function turnTimeTitle(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return fullTitle(new Date(then))
}
