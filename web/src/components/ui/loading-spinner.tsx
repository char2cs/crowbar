import type React from 'react'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

interface LoadingSpinnerProps {
  label?: string
  showLabel?: boolean
  compact?: boolean
  className?: string
}

export function LoadingSpinner({
  label = 'Loading',
  showLabel,
  compact,
  className,
}: LoadingSpinnerProps): React.ReactElement {
  return (
    <span
      className={cn('inline-flex items-center gap-1.5', compact && 'gap-1', className)}
      data-oracle-content-sized="true"
      data-oracle-id="loading-spinner"
    >
      <Spinner className={cn('size-4', compact && 'size-3')} aria-label={label} />
      {showLabel && (
        <span
          className="text-muted-foreground ui-text-sm"
          data-oracle-content-sized="true"
          data-oracle-id="loading-spinner-label"
          data-oracle-line-sized="true"
        >
          {label}
        </span>
      )}
    </span>
  )
}
