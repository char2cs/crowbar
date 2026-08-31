import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import type { SwitchKind } from '@/features/agent/transcript/switch-divider'

/** One row of the transcript's flat, virtualizable list. `trailingInterruption`,
 *  streaming bubbles, the queue and the working line are not rows here — they
 *  sit outside `messages` in agent-transcript.tsx and stay owned by it. */
export type TranscriptRow =
  | { kind: 'message'; key: string; message: AgentChatMessage }
  | {
      kind: 'compaction-divider'
      key: string
      sequence: number
      trigger: 'manual' | 'auto' | string
    }
  | { kind: 'interrupted-divider'; key: string; sequence: number }
  | { kind: 'switch-divider'; key: string; sequence: number; what: SwitchKind; detail: string }
  | { kind: 'first-turn-divider'; key: string }

/**
 * Mirrors agent-transcript.tsx's `messages.map(...)` block: per message,
 * compaction divider, interrupted divider, switch dividers (provider/model/
 * effort — Crowbar's own doing, see switch-divider.tsx), then the message,
 * then (only on `firstTurnSequence`) the first-turn divider.
 * `switchesBefore` is an ARRAY per sequence, not a single value: a
 * SetChatSelection call can change model AND effort together, and both need
 * their own divider before the same next message. `suppressSequence` drops
 * that whole group, dividers included, same as the ternary wrapping the
 * fragment in the real render. No `firstReplySequence` param — in the
 * current render it only sets MessageRow's presentational `firstReply` prop
 * and never gates a row's presence or position.
 */
export function flattenTranscriptRows({
  messages,
  compactionBefore,
  interruptedBefore,
  switchesBefore,
  firstTurnSequence,
  suppressSequence,
}: {
  messages: AgentChatMessage[]
  compactionBefore?: Record<number, 'manual' | 'auto' | string>
  interruptedBefore?: Record<number, true>
  switchesBefore?: Record<number, Array<{ what: SwitchKind; detail: string }>>
  firstTurnSequence: number | undefined
  suppressSequence?: number
}): TranscriptRow[] {
  const rows: TranscriptRow[] = []

  for (const message of messages) {
    if (message.sequence === suppressSequence) continue

    const trigger = compactionBefore?.[message.sequence]
    if (trigger) {
      rows.push({
        kind: 'compaction-divider',
        key: `compaction-${message.sequence}`,
        sequence: message.sequence,
        trigger,
      })
    }

    if (interruptedBefore?.[message.sequence]) {
      rows.push({
        kind: 'interrupted-divider',
        key: `interrupted-${message.sequence}`,
        sequence: message.sequence,
      })
    }

    const switches = switchesBefore?.[message.sequence]
    if (switches) {
      switches.forEach((s, i) => {
        rows.push({
          kind: 'switch-divider',
          key: `switch-${message.sequence}-${i}`,
          sequence: message.sequence,
          what: s.what,
          detail: s.detail,
        })
      })
    }

    rows.push({ kind: 'message', key: `message-${message.sequence}`, message })

    if (message.sequence === firstTurnSequence) {
      rows.push({ kind: 'first-turn-divider', key: 'first-turn-divider' })
    }
  }

  return rows
}
