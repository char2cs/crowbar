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
