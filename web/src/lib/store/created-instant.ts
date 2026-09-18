/** What Go's zero time.Time marshals as — a repo Node's CreatedAt. */
const ZERO_TIME = Date.parse('0001-01-01T00:00:00Z')

/**
 * A row's `createdAt` as the instant the daemon compares (time.Compare), not
 * the ISO string: Go trims fraction zeros and stamps a local offset, so the
 * string order is not the instant order. Absent or unparsable sorts first,
 * like a zero time.
 */
export function createdInstant(createdAt: string | undefined): number {
  const t = createdAt ? Date.parse(createdAt) : NaN
  return Number.isNaN(t) ? ZERO_TIME : t
}
