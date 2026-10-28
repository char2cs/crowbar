import type { ReactNode } from 'react'
import { DesktopIcon } from '@phosphor-icons/react'
import { hostDotLabel, hostTone, type HostTone } from '@/features/hosts/lib/host-tone'
import type { HostDotState } from '@/features/hosts/types'
import { cn } from '@/utils/cn'

// A dot is a non-text indicator, so it needs 3:1. Light amber's base is 2.2:1 on the sidebar, so
// light takes its -foreground (as the tone's text does); the unsettled ring is /65 for the same.
const DOT_FILL: Record<HostTone, string> = {
  online: 'bg-success',
  offline: 'bg-destructive',
  'needs-attention': 'bg-warning-foreground dark:bg-warning',
  unsettled: 'border border-muted-foreground/65',
}

// The dot sits over the surface's corner: the surface is cut away around it, so the dot reads on
// anything (the sidebar is translucent, so no ring colour could match what shows through it).
const CUT_AROUND_DOT =
  '[mask-image:radial-gradient(circle_at_calc(100%_-_1.5px)_calc(100%_-_1.5px),transparent_3.75px,black_4.25px)]'

/** `children` with its host's status dot on the bottom-right corner. Every mark of a host's
 *  status draws this dot, so a state reads the same wherever it shows. */
export function HostStatusDot({
  hostId,
  state,
  labelled = false,
  showTitle = false,
  children,
}: {
  hostId: string
  state: HostDotState
  /** Says the state in words; off where the surface's own label already says it. */
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

/** The status dot on its own, beside a label that names the host; the state is said in words to
 *  assistive tech, and colour carries it on screen. */
export function HostDot({ state }: { state: HostDotState }) {
  const tone = hostTone(state)
  return (
    <span
      data-host-state={state}
      data-host-tone={tone}
      role="img"
      aria-label={hostDotLabel(state)}
      className={cn('size-1.5 shrink-0 rounded-full', DOT_FILL[tone])}
    />
  )
}

/** A computer with its host's status dot, the dot labelled in words. */
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
  /** Off inside a control that has a tooltip of its own, so two don't stack. */
  showTitle?: boolean
  /** Sizes the icon (`size-4` by default); the dot keeps its own size. */
  className?: string
}) {
  return (
    <HostStatusDot hostId={hostId} state={state} labelled showTitle={showTitle}>
      <DesktopIcon aria-hidden="true" weight={weight} className={cn('size-4', className)} />
    </HostStatusDot>
  )
}
