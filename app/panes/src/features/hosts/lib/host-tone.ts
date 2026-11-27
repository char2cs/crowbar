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

export function hostDotLabel(state: HostDotState): string {
  return LABEL[state]
}

const UNREACHABLE: ReadonlySet<HostDotState> = new Set(['down', 'needs-repair', 'not-available'])

export function hostIsUnreachable(state: HostDotState): boolean {
  return UNREACHABLE.has(state)
}

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

const TONE_TEXT: Record<HostTone, string> = {
  online:
    'text-success-foreground hover:text-success-foreground dark:text-success dark:hover:text-success',
  offline:
    'text-destructive-foreground hover:text-destructive-foreground dark:text-destructive dark:hover:text-destructive',
  'needs-attention':
    'text-warning-foreground hover:text-warning-foreground dark:text-warning dark:hover:text-warning',
  unsettled: 'text-muted-foreground hover:text-muted-foreground',
}

export function hostToneText(state: HostDotState, className?: string) {
  return {
    'data-host-state': state,
    'data-host-tone': TONE[state],
    className: cn(className, TONE_TEXT[TONE[state]]),
  }
}
