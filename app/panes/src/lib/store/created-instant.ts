const ZERO_TIME = Date.parse('0001-01-01T00:00:00Z')

export function createdInstant(createdAt: string | undefined): number {
  const t = createdAt ? Date.parse(createdAt) : NaN
  return Number.isNaN(t) ? ZERO_TIME : t
}
