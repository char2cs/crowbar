import { formatRelativeDate } from '@/utils/date'

interface StaleBannerProps {
  at: number | null
  onRetry: () => void
  isRefreshing: boolean
}

export function StaleBanner({ at, onRetry, isRefreshing }: StaleBannerProps) {
  return (
    <div className="flex items-center gap-2 border-b border-warning/30 bg-warning/10 px-3 py-1 text-[11px] text-warning">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-warning" />
      <span>
        Showing cached data
        {at !== null ? ` · last updated ${formatRelativeDate(new Date(at))}` : ''}
      </span>
      {isRefreshing ? (
        <span className="opacity-70">· Refreshing…</span>
      ) : (
        <button type="button" onClick={onRetry} className="underline hover:text-warning-foreground">
          Retry
        </button>
      )}
    </div>
  )
}
