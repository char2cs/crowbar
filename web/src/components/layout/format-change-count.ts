/**
 * Formats an added/deleted line count for compact display: values over 999
 * are rounded to a "k" suffix (e.g. 1500 → "2k"). Mirrors the formatting
 * used in the workspace tree so the switcher and tree render counts identically.
 */
export function formatChangeCount(n: number): string {
  return n > 999 ? `${Math.round(n / 1000)}k` : `${n}`
}
