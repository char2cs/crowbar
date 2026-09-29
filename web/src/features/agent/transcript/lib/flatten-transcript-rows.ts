import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import type { ActivityComponent } from '@/features/agent/lib/activity-components'

/** One boundary event drawn as a pill inside an `event-divider` row. Several
 *  can land before the same next message — a stop followed by a provider
 *  switch, or a SetChatSelection call changing model AND effort together —
 *  and all of them collapse into pills on the SAME wavy line rather than one
 *  full-width divider each. Order within the array is chronological; it is
 *  the caller's job (agent-chat-view.tsx has the real timestamps) to sort
 *  before grouping, not this file's. */
export type DividerTag =
  | { kind: 'compaction'; id: string; trigger: 'manual' | 'auto' | string }
  | { kind: 'interrupted'; id: string }
  | { kind: 'inferred-interrupt'; id: string }
  | { kind: 'provider' | 'model' | 'effort'; id: string; detail: string }

/** One row of the transcript's flat, virtualizable list. `trailingInterruption`,
 *  the queue and the working line remain tail items owned by agent-transcript. */
export type TranscriptRow =
  | { kind: 'message'; key: string; message: AgentChatMessage; streaming?: boolean }
  | { kind: 'activity'; key: string; component: ActivityComponent }
  | { kind: 'event-divider'; key: string; sequence: number; tags: DividerTag[] }
  | { kind: 'first-turn-divider'; key: string }

/**
 * Merge durable messages, streaming messages and activity components into one
 * chronological, virtualizable list. Activity updates retain their component
 * ID and createdAt, so a status update changes the existing row without moving
 * it. Message-shaped components mirror messages and are intentionally omitted;
 * tool output and diff children are rendered inside their owning activity row.
 */
export function flattenTranscriptRows({
  messages,
  streamingMessages = [],
  components = [],
  excludeComponentIds,
  eventsBefore,
  firstTurnSequence,
  suppressSequence,
}: {
  messages: AgentChatMessage[]
  streamingMessages?: AgentChatMessage[]
  components?: ActivityComponent[]
  /** Components rendered inside an owning row (for example a turn diff inside
   * its edit tool) remain canonical records but do not get a duplicate row. */
  excludeComponentIds?: ReadonlySet<string>
  eventsBefore?: Record<number, DividerTag[]>
  firstTurnSequence: number | undefined
  suppressSequence?: number
}): TranscriptRow[] {
  const seenSequences = new Set<number>()
  const streaming = new Set(streamingMessages)
  const entries: Array<{
    at: string
    order: number
    key: string
    row: TranscriptRow
  }> = []

  for (const message of [...messages, ...streamingMessages]) {
    if (message.sequence === suppressSequence) continue
    if (seenSequences.has(message.sequence)) continue
    seenSequences.add(message.sequence)

    const key = `message-${message.sequence}`
    entries.push({
      at: message.at,
      order: message.displayOrder ?? message.sequence,
      key,
      row: { kind: 'message', key, message, streaming: streaming.has(message) },
    })
  }

  for (const component of components) {
    if (excludeComponentIds?.has(component.id)) continue
    if (
      component.kind === 'user_message' ||
      component.kind === 'assistant_message' ||
      component.kind === 'tool_output' ||
      component.kind === 'status_notice'
    ) {
      continue
    }
    const key = `activity-${component.id}`
    entries.push({
      at: component.createdAt,
      order: component.seq,
      key,
      row: { kind: 'activity', key, component },
    })
  }

  entries.sort((a, b) => {
    const aMessage = a.row.kind === 'message' ? a.row.message : undefined
    const bMessage = b.row.kind === 'message' ? b.row.message : undefined
    const aComponent = a.row.kind === 'activity' ? a.row.component : undefined
    const bComponent = b.row.kind === 'activity' ? b.row.component : undefined
    if (
      aMessage?.role === 'assistant' &&
      bComponent?.turnId === aMessage.turnId &&
      aComponent === undefined
    ) {
      return 1
    }
    if (
      bMessage?.role === 'assistant' &&
      aComponent?.turnId === bMessage.turnId &&
      bComponent === undefined
    ) {
      return -1
    }
    const aAt = Date.parse(a.at)
    const bAt = Date.parse(b.at)
    if (!Number.isNaN(aAt) && !Number.isNaN(bAt) && aAt !== bAt) return aAt - bAt
    if (a.order !== b.order) return a.order - b.order
    return a.key.localeCompare(b.key)
  })

  const rows: TranscriptRow[] = []
  for (const { row } of entries) {
    if (row.kind === 'message') {
      const { sequence } = row.message
      const tags = eventsBefore?.[sequence]
      if (tags && tags.length > 0) {
        rows.push({
          kind: 'event-divider',
          key: `events-${sequence}`,
          sequence,
          tags,
        })
      }
      rows.push(row)
      if (row.message.sequence === firstTurnSequence) {
        rows.push({ kind: 'first-turn-divider', key: 'first-turn-divider' })
      }
    } else {
      rows.push(row)
    }
  }

  return rows
}
