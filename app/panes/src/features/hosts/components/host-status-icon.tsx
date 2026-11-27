import type { ReactNode } from 'react'
import { CloudIcon, DesktopIcon } from '@phosphor-icons/react'
import { hostDotLabel, hostTone, type HostTone } from '@/features/hosts/lib/host-tone'
import { LOCAL_HOST, type HostDotState } from '@/features/hosts/types'
import { cn } from '@/utils/cn'

const DOT_FILL: Record<HostTone, string> = {
  online: 'bg-success',
  offline: 'bg-destructive',
  'needs-attention': 'bg-warning-foreground dark:bg-warning',
  unsettled: 'border border-muted-foreground/65',
}

const CUT_AROUND_DOT =
  '[mask-image:radial-gradient(circle_at_calc(100%_-_1.5px)_calc(100%_-_1.5px),transparent_3.75px,black_4.25px)]'

export function HostStatusDot({
  hostId,
  state,
  labelled = false,
  showTitle = false,
  children,
}: {
  hostId: string
  state: HostDotState
  labelled?: boolean
  showTitle?: boolean
  children: ReactNode
}) {
  const tone = hostTone(state)
  const words = hostDotLabel(state)
  return (
    <span className="relative inline-flex shrink-0">
      <span className={cn('flex', CUT_AROUND_DOT)}>{children}</span>
      <span
        data-testid={`host-status-${hostId}`}
        data-host-state={state}
        data-host-tone={tone}
        {...(labelled ? { role: 'img', 'aria-label': words } : { 'aria-hidden': true })}
        title={labelled && showTitle ? words : undefined}
        className={cn('absolute -right-px -bottom-px size-1.25 rounded-full', DOT_FILL[tone])}
      />
    </span>
  )
}

export function hostPlace(hostId: string): 'local' | 'remote' {
  return hostId === LOCAL_HOST.id ? 'local' : 'remote'
}

export function HostStatusIcon({
  hostId,
  state,
  weight = 'regular',
  showTitle = true,
  className,
}: {
  hostId: string
  state: HostDotState
  weight?: 'regular' | 'bold'
  showTitle?: boolean
  className?: string
}) {
  const place = hostPlace(hostId)
  const Icon = place === 'local' ? DesktopIcon : CloudIcon
  return (
    <HostStatusDot hostId={hostId} state={state} labelled showTitle={showTitle}>
      <Icon
        aria-hidden="true"
        data-host-place={place}
        weight={weight}
        className={cn('size-4', className)}
      />
    </HostStatusDot>
  )
}
