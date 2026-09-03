import type { AgentChatMessage } from '@/features/agent/api/agent-api'

/** One boundary event drawn as a pill inside an `event-divider` row. Several
 *  can land before the same next message — a stop followed by a provider
 *  switch, or a SetChatSelection call changing model AND effort together —
 *  and all of them collapse into pills on the SAME wavy line rather than one
 *  full-width divider each. Order within the array is chronological; it is
 *  the caller's job (agent-chat-view.tsx has the real timestamps) to sort
 *  before grouping, not this file's. */
export type DividerTag =
  | { kind: 'compaction'; trigger: 'manual' | 'auto' | string }
  | { kind: 'interrupted' }
  | { kind: 'provider' | 'model' | 'effort'; detail: string }

/** One row of the transcript's flat, virtualizable list. `trailingInterruption`,
 *  streaming bubbles, the queue and the working line are not rows here — they
 *  sit outside `messages` in agent-transcript.tsx and stay owned by it. */
export type TranscriptRow =
  | { kind: 'message'; key: string; message: AgentChatMessage }
  | { kind: 'event-divider'; key: string; sequence: number; tags: DividerTag[] }
  | { kind: 'first-turn-divider'; key: string }

/**
 * Mirrors agent-transcript.tsx's `messages.map(...)` block: per message, the
 * event divider (if anything landed before it), then the message, then
 * (only on `firstTurnSequence`) the first-turn divider. `suppressSequence`
 * drops that whole group, divider included, same as the ternary wrapping the
 * fragment in the real render.
 */
export function flattenTranscriptRows({
  messages,
  eventsBefore,
  firstTurnSequence,
  suppressSequence,
}: {
  messages: AgentChatMessage[]
  eventsBefore?: Record<number, DividerTag[]>
  firstTurnSequence: number | undefined
  suppressSequence?: number
}): TranscriptRow[] {
  const rows: TranscriptRow[] = []

  for (const message of messages) {
    if (message.sequence === suppressSequence) continue

    const tags = eventsBefore?.[message.sequence]
    if (tags && tags.length > 0) {
      rows.push({
        kind: 'event-divider',
        key: `events-${message.sequence}`,
        sequence: message.sequence,
        tags,
      })
    }

    rows.push({ kind: 'message', key: `message-${message.sequence}`, message })

    if (message.sequence === firstTurnSequence) {
      rows.push({ kind: 'first-turn-divider', key: 'first-turn-divider' })
    }
  }

  return rows
}
