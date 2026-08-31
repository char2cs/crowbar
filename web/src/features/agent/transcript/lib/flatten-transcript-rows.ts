import type { AgentChatMessage } from '@/features/agent/api/agent-api'

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
  | { kind: 'first-turn-divider'; key: string }

/**
 * Mirrors agent-transcript.tsx's `messages.map(...)` block: per message,
 * compaction divider then interrupted divider then the message then (only on
 * `firstTurnSequence`) the first-turn divider. `suppressSequence` drops that
 * whole group, dividers included, same as the ternary wrapping the fragment
 * in the real render. No `firstReplySequence` param — in the current render
 * it only sets MessageRow's presentational `firstReply` prop and never gates
 * a row's presence or position.
 */
export function flattenTranscriptRows({
  messages,
  compactionBefore,
  interruptedBefore,
  firstTurnSequence,
  suppressSequence,
}: {
  messages: AgentChatMessage[]
  compactionBefore?: Record<number, 'manual' | 'auto' | string>
  interruptedBefore?: Record<number, true>
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

    rows.push({ kind: 'message', key: `message-${message.sequence}`, message })

    if (message.sequence === firstTurnSequence) {
      rows.push({ kind: 'first-turn-divider', key: 'first-turn-divider' })
    }
  }

  return rows
}
