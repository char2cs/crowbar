import type { ReactNode } from 'react'
import { type Loadable, dataOf, fetchedAtOf } from '@/lib/loadable'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { InlineError } from '@/components/ui/inline-error'
import { StaleBanner } from '@/components/ui/stale-banner'

interface DataStateProps<T> {
  loadable: Loadable<T>
  onRetry: () => void
  children: (data: T) => ReactNode
  loadingLabel?: string
  emptyMessage?: string
  isEmpty?: (data: T) => boolean
}

export function DataState<T>({
  loadable, onRetry, children, loadingLabel, emptyMessage, isEmpty,
}: DataStateProps<T>) {
  const stale = dataOf(loadable)

  if (stale === undefined) {
    if (loadable.status === 'loading') return <LoadingSpinner label={loadingLabel} />
    if (loadable.status === 'error') return <InlineError error={loadable.error} onRetry={onRetry} />
    return null
  }

  const data = loadable.status === 'success' ? loadable.data : stale
  const showBanner = loadable.status !== 'success'
  const empty = (isEmpty ?? ((d: T) => Array.isArray(d) && d.length === 0))(data)

  return (
    <>
      {showBanner && (
        <StaleBanner
          at={fetchedAtOf(loadable) ?? null}
          onRetry={onRetry}
          isRefreshing={loadable.status === 'loading'}
        />
      )}
      {empty && emptyMessage ? (
        <p className="p-6 text-center text-sm text-muted-foreground">{emptyMessage}</p>
      ) : (
        children(data)
      )}
    </>
  )
}
