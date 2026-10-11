import type { HostDotState } from '@/features/hosts/types'
import { cn } from '@/utils/cn'

const LABEL: Record<HostDotState, string> = {
  unknown: 'Status unknown',
  probing: 'Checking…',
  up: 'Online',
  down: 'Offline',
  'needs-repair': 'Needs repair — reconnect this host',
  'no-live-stream': 'Online — no live stream',
  'not-available': 'Host not available',
}

const UNSETTLED = 'border border-muted-foreground/50 bg-transparent'
const UP = 'bg-success shadow-[0_0_4px_--theme(--color-success/60%)]'
// Spec §5.1: a host that does not answer is a muted grey, not an alarm.
const NOT_ANSWERING = 'bg-muted-foreground/50'

const DOT_CLASS: Record<HostDotState, string> = {
  unknown: UNSETTLED,
  probing: UNSETTLED,
  up: UP,
  'no-live-stream': UP,
  down: NOT_ANSWERING,
  'needs-repair': 'bg-warning',
  'not-available': NOT_ANSWERING,
}

/** The words for `state`, for a surface that says it in text as well as on the dot. */
export function hostDotLabel(state: HostDotState): string {
  return LABEL[state]
}

/** A host's status dot, labelled for what it says. Every surface that shows a host's status
 *  draws this one, so a state reads the same wherever it appears. */
export function HostStatusDot({
  hostId,
  state,
  className,
}: {
  hostId: string
  state: HostDotState
  className?: string
}) {
  return (
    <span
      data-testid={`host-status-${hostId}`}
      aria-label={LABEL[state]}
      title={LABEL[state]}
      className={cn('size-2 shrink-0 rounded-full', DOT_CLASS[state], className)}
    />
  )
}
