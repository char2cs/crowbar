/** Stable React keys for id-less, append-only lists: identical entries are told apart by occurrence. */
export function occurrenceKeys<T>(items: readonly T[], identity: (item: T) => string): string[] {
  const seen = new Map<string, number>()
  return items.map((item) => {
    const base = identity(item)
    const count = seen.get(base) ?? 0
    seen.set(base, count + 1)
    return `${base}#${count}`
  })
}
