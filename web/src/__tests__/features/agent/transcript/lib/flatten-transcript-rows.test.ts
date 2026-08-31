import { describe, expect, it } from 'vitest'

import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { flattenTranscriptRows } from '@/features/agent/transcript/lib/flatten-transcript-rows'

function msg(sequence: number, role: AgentChatMessage['role'] = 'user'): AgentChatMessage {
  return { sequence, turnId: `t${sequence}`, role, providerId: '', text: `msg ${sequence}`, at: '' }
}

describe('flattenTranscriptRows', () => {
  it('emits one row per message, in sequence order', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      compactionBefore: {},
      interruptedBefore: {},
      firstTurnSequence: undefined,
    })

    const messageRows = rows.filter((r) => r.kind === 'message')
    expect(messageRows).toHaveLength(3)
    expect(messageRows.map((r) => r.message.sequence)).toEqual([1, 2, 3])
  })

  it('inserts a compaction-divider row before the message it precedes', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: { 2: 'manual' },
      interruptedBefore: {},
      firstTurnSequence: undefined,
    })

    const idx = rows.findIndex((r) => r.kind === 'compaction-divider')
    expect(idx).toBeGreaterThanOrEqual(0)
    expect(rows[idx]).toMatchObject({ kind: 'compaction-divider', sequence: 2, trigger: 'manual' })
    expect(rows[idx + 1]).toMatchObject({ kind: 'message', message: { sequence: 2 } })
  })

  it('inserts an interrupted-divider row before the message it precedes', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: {},
      interruptedBefore: { 2: true },
      firstTurnSequence: undefined,
    })

    const idx = rows.findIndex((r) => r.kind === 'interrupted-divider')
    expect(idx).toBeGreaterThanOrEqual(0)
    expect(rows[idx]).toMatchObject({ kind: 'interrupted-divider', sequence: 2 })
    expect(rows[idx + 1]).toMatchObject({ kind: 'message', message: { sequence: 2 } })
  })

  it('renders compaction before interrupted when both precede the same message', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: { 2: 'auto' },
      interruptedBefore: { 2: true },
      firstTurnSequence: undefined,
    })

    const kinds = rows.map((r) => r.kind)
    expect(kinds).toEqual(['message', 'compaction-divider', 'interrupted-divider', 'message'])
  })

  it('inserts a first-turn-divider row after the message matching firstTurnSequence', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: {},
      interruptedBefore: {},
      firstTurnSequence: 1,
    })

    const msgIdx = rows.findIndex((r) => r.kind === 'message' && r.message.sequence === 1)
    expect(rows[msgIdx + 1]?.kind).toBe('first-turn-divider')
    expect(rows.filter((r) => r.kind === 'first-turn-divider')).toHaveLength(1)
  })

  it('omits the first-turn-divider when firstTurnSequence matches nothing', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: {},
      interruptedBefore: {},
      firstTurnSequence: undefined,
    })

    expect(rows.some((r) => r.kind === 'first-turn-divider')).toBe(false)
  })

  it('drops a suppressed message and its dividers entirely', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      compactionBefore: { 2: 'manual' },
      interruptedBefore: { 2: true },
      firstTurnSequence: 2,
      suppressSequence: 2,
    })

    expect(rows.map((r) => r.kind)).toEqual(['message', 'message'])
    expect(rows.map((r) => (r.kind === 'message' ? r.message.sequence : undefined))).toEqual([1, 3])
  })

  it('gives every row a unique key', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: { 2: 'manual' },
      interruptedBefore: { 2: true },
      firstTurnSequence: 1,
    })

    const keys = rows.map((r) => r.key)
    expect(new Set(keys).size).toBe(keys.length)
  })
})
