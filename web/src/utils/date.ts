const relativeTimeFormatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const dateFormatterUS = new Intl.DateTimeFormat('en-US')

/**
 * Format a Unix timestamp (seconds) to a relative time string.
 * Returns "3 hours ago", "2 days ago", "last month", etc.
 */
export const formatRelativeTime = (timestamp: number) => {
  const date = new Date(timestamp * 1000)
  const diff = (Date.now() - date.getTime()) / 1000

  if (diff < 60) return relativeTimeFormatter.format(-Math.round(diff), 'second')
  if (diff < 3600) return relativeTimeFormatter.format(-Math.round(diff / 60), 'minute')
  if (diff < 86400) return relativeTimeFormatter.format(-Math.round(diff / 3600), 'hour')
  if (diff < 2592000) return relativeTimeFormatter.format(-Math.round(diff / 86400), 'day')
  return relativeTimeFormatter.format(-Math.round(diff / 2592000), 'month')
}

/**
 * Format a date string to a compact relative time format.
 * Returns "just now", "5m ago", "2h ago", "yesterday", "3d ago", or locale date.
 */
export const formatRelativeDate = (dateString: string | Date): string => {
  const date = new Date(dateString)
  if (Number.isNaN(date.getTime())) {
    return typeof dateString === 'string' ? dateString : 'unknown'
  }

  const diffMs = Date.now() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays === 1) return 'yesterday'
  if (diffDays < 7) return `${diffDays}d ago`

  return dateFormatterUS.format(date)
}
