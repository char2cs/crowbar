import type { HostDotState } from '@/features/hosts/types'
import { cn } from '@/utils/cn'

const LABEL: Record<HostDotState, string> = {
  unknown: 'Status unknown',
  probing: 'Checking…',
  up: 'Online',
  down: 'Offline',
  'needs-repair': 'Needs pairing again',
  'no-live-stream': 'Online — no live stream',
  'not-available': 'Host not available',
}

/** The words for `state`: every surface that colours a host's state says it in these too. */
export function hostDotLabel(state: HostDotState): string {
  return LABEL[state]
}

const UNREACHABLE: ReadonlySet<HostDotState> = new Set(['down', 'needs-repair', 'not-available'])

/** Whether nothing can be asked of a host in `state` now: it does not answer, it refuses this
 *  client until paired again, or it is gone. */
export function hostIsUnreachable(state: HostDotState): boolean {
  return UNREACHABLE.has(state)
}

/** The colour a host's state is drawn in, wherever it shows. */
export type HostTone = 'online' | 'offline' | 'needs-attention' | 'unsettled'

const TONE: Record<HostDotState, HostTone> = {
  up: 'online',
  'no-live-stream': 'online',
  down: 'offline',
  'not-available': 'offline',
  'needs-repair': 'needs-attention',
  unknown: 'unsettled',
  probing: 'unsettled',
}

export function hostTone(state: HostDotState): HostTone {
  return TONE[state]
}

// Light text takes each status colour's darker -foreground: the base tokens fall under WCAG AA
// on the sidebar glass. Dark's -foreground is the ink drawn ON a status fill, so dark keeps the base.
// Each is repeated under hover: so a caller's own hover colour can't turn the tone neutral.
const TONE_TEXT: Record<HostTone, string> = {
  online:
    'text-success-foreground hover:text-success-foreground dark:text-success dark:hover:text-success',
  offline:
    'text-destructive-foreground hover:text-destructive-foreground dark:text-destructive dark:hover:text-destructive',
  'needs-attention':
    'text-warning-foreground hover:text-warning-foreground dark:text-warning dark:hover:text-warning',
  unsettled: 'text-muted-foreground hover:text-muted-foreground',
}

/** Props that tint an element's text in `state`'s tone and name the state it shows. Colour is
 *  never the only carrier: the element, or its row, says the state in words too. */
export function hostToneText(state: HostDotState, className?: string) {
  return {
    'data-host-state': state,
    'data-host-tone': TONE[state],
    className: cn(className, TONE_TEXT[TONE[state]]),
  }
}
