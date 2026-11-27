import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import { describeFailure, type FailureKind } from '@/features/agent/transcript/lib/turn-failure'

export type NoticeTone = 'warn' | 'limit' | 'danger'

export interface NoticeActions {
  compact?: () => void
  openChat: (chatId: string) => void
}

export interface FailureActions {
  retry: (requestId: string) => void
  openTerminal: () => void
  newConversation: () => void
}

type FailureWay = 'retry' | 'sign-in' | 'switch-model' | 'new-conversation' | 'none'

export interface MarkerFailure {
  tone: NoticeTone
  headline: string
  hint?: string
  tag: string
  way: FailureWay
}

export const stoppedByYou = (reason: string | undefined) =>
  reason === 'stopped' || reason === 'retracted'

const TONE: Record<FailureKind, NoticeTone> = {
  auth: 'warn',
  overloaded: 'warn',
  limit: 'limit',
  other: 'danger',
}

const WAY: Record<FailureKind, FailureWay> = {
  auth: 'sign-in',
  limit: 'switch-model',
  overloaded: 'retry',
  other: 'retry',
}

export interface MarkerFailureContext {
  provider?: string
  resetsAt?: string
  now?: Date
}

const capitalised = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

export function markerFailure(
  item: MarkerItem,
  { provider, resetsAt, now }: MarkerFailureContext = {},
): MarkerFailure | undefined {
  switch (item.marker) {
    case 'turn_failed': {
      const said = describeFailure({
        kind: item.failureKind,
        provider,
        resetsAt,
        now,
      })
      return { tone: TONE[said.kind], way: WAY[said.kind], ...said }
    }
    case 'session_lost':
      return {
        tone: 'danger',
        headline: 'The session was lost',
        hint: 'Start a new conversation to keep going.',
        tag: 'Stopped · session lost',
        way: 'new-conversation',
      }
    case 'runner_exited': {
      if (item.reason !== 'spawn_failed') return undefined
      const headline = `${capitalised(provider || 'the agent')} could not start`
      return {
        tone: 'danger',
        headline,
        hint: item.detail ?? 'Check that its CLI is installed.',
        tag: `Stopped · ${headline}${item.detail ? `: ${item.detail}` : ''}`,
        way: 'none',
      }
    }
    case 'turn_interrupted': {
      if (stoppedByYou(item.reason)) return undefined
      return {
        tone: 'warn',
        headline: 'The turn was cut short',
        hint: 'What was said is kept. Try again to carry on.',
        tag: item.reason === 'provider_idle' ? 'Interrupted unexpectedly' : 'Interrupted',
        way: 'retry',
      }
    }
    default:
      return undefined
  }
}
