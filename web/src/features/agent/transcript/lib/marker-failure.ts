import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import { describeFailure, type FailureKind } from '@/features/agent/transcript/lib/turn-failure'

/** How loud the tray above the input box reads: a sign-in or a busy provider asks for
 *  attention, a limit is plain, a failure is red. */
export type NoticeTone = 'warn' | 'limit' | 'danger'

/** What a notice in the transcript can do. */
export interface NoticeActions {
  /** Absent while compacting is not on offer (a turn is running, or the provider cannot). */
  compact?: () => void
  /** Shows the chat `chatId`, where it is already open or in a new tab. */
  openChat: (chatId: string) => void
}

/** What a stop's notice can do. */
export interface FailureActions {
  /** Sends the prompt `requestId` again, as its retry. */
  retry: (requestId: string) => void
  openTerminal: () => void
  /** The next prompt starts a fresh conversation. */
  newConversation: () => void
}

/** What the person can do about a stopped turn, by what stopped it. */
type FailureWay = 'retry' | 'sign-in' | 'switch-model' | 'new-conversation' | 'none'

export interface MarkerFailure {
  tone: NoticeTone
  headline: string
  hint?: string
  /** The divider the transcript keeps once the turn is behind. */
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
  /** The provider's display name; "the agent" when unknown. */
  provider?: string
  resetsAt?: string
  now?: Date
}

const capitalised = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

/** A marker that stopped the chat in a way the person has to hear about, in the board's
 *  words; undefined for every boundary that needs none. */
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
        hint: 'Check that its CLI is installed.',
        tag: `Stopped · ${headline}`,
        way: 'none',
      }
    }
    case 'turn_interrupted': {
      if (stoppedByYou(item.reason)) return undefined
      return {
        tone: 'warn',
        headline: 'The turn was cut short',
        hint: 'What was said is kept. Try again to carry on.',
        // Closed on the provider going quiet, not by anyone: never worded as the user's doing.
        tag: item.reason === 'provider_idle' ? 'Interrupted unexpectedly' : 'Interrupted',
        way: 'retry',
      }
    }
    default:
      return undefined
  }
}
