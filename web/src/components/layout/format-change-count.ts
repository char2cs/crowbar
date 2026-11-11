/** A line count for compact display, rounded to "k" past 999 (1500 → "2k"). */
export function formatChangeCount(n: number): string {
  return n > 999 ? `${Math.round(n / 1000)}k` : `${n}`
}
