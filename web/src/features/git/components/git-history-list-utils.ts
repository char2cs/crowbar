import { formatRelativeTime } from '@/utils/date'

// commitDateLabel renders the backend's ISO commit date as a relative time
// ("2 hours ago"); an unparseable date falls back to the raw string.
export function commitDateLabel(isoDate: string): string {
  const ms = Date.parse(isoDate)
  if (Number.isNaN(ms)) return isoDate
  return formatRelativeTime(ms / 1000)
}

// nearListEnd reports whether the viewport has scrolled past 80% of the list —
// the threshold at which the next commit page is requested.
export function nearListEnd(
  scrollTop: number,
  clientHeight: number,
  scrollHeight: number,
): boolean {
  if (scrollHeight <= 0) return false
  return (scrollTop + clientHeight) / scrollHeight >= 0.8
}
