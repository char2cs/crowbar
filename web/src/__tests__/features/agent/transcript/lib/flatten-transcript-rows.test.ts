import { describe, expect, it } from 'vitest'

import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import {
  flattenTranscriptRows,
  type DividerTag,
} from '@/features/agent/transcript/lib/flatten-transcript-rows'

function msg(sequence: number, role: AgentChatMessage['role'] = 'user'): AgentChatMessage {
  return { sequence, turnId: `t${sequence}`, role, providerId: '', text: `msg ${sequence}`, at: '' }
}

describe('flattenTranscriptRows', () => {
  it('emits one row per message, in sequence order', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      eventsBefore: {},
      firstTurnSequence: undefined,
    })

    const messageRows = rows.filter((r) => r.kind === 'message')
    expect(messageRows).toHaveLength(3)
    expect(messageRows.map((r) => r.message.sequence)).toEqual([1, 2, 3])
  })

  it('inserts an event-divider row before the message it precedes', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: { 2: [{ kind: 'compaction', trigger: 'manual' }] },
      firstTurnSequence: undefined,
    })

    const idx = rows.findIndex((r) => r.kind === 'event-divider')
    expect(idx).toBeGreaterThanOrEqual(0)
    expect(rows[idx]).toMatchObject({
      kind: 'event-divider',
      sequence: 2,
      tags: [{ kind: 'compaction', trigger: 'manual' }],
    })
    expect(rows[idx + 1]).toMatchObject({ kind: 'message', message: { sequence: 2 } })
  })

  it('collapses several tags for the same anchor into ONE row, in the order given', () => {
    const tags: DividerTag[] = [
      { kind: 'interrupted' },
      { kind: 'provider', detail: 'codex' },
      { kind: 'model', detail: 'opus' },
      { kind: 'effort', detail: 'high' },
    ]
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: { 2: tags },
      firstTurnSequence: undefined,
    })

    const dividerRows = rows.filter((r) => r.kind === 'event-divider')
    expect(dividerRows).toHaveLength(1)
    expect(dividerRows[0]).toMatchObject({ sequence: 2, tags })
  })

  it('draws nothing for an anchor with an empty tag list', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: { 2: [] },
      firstTurnSequence: undefined,
    })

    expect(rows.some((r) => r.kind === 'event-divider')).toBe(false)
  })

  it('drops a suppressed message and its event divider entirely', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      eventsBefore: { 2: [{ kind: 'provider', detail: 'codex' }] },
      firstTurnSequence: undefined,
      suppressSequence: 2,
    })

    expect(rows.map((r) => r.kind)).toEqual(['message', 'message'])
  })

  it('inserts a first-turn-divider row after the message matching firstTurnSequence', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: {},
      firstTurnSequence: 1,
    })

    const msgIdx = rows.findIndex((r) => r.kind === 'message' && r.message.sequence === 1)
    expect(rows[msgIdx + 1]?.kind).toBe('first-turn-divider')
    expect(rows.filter((r) => r.kind === 'first-turn-divider')).toHaveLength(1)
  })

  it('omits the first-turn-divider when firstTurnSequence matches nothing', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: {},
      firstTurnSequence: undefined,
    })

    expect(rows.some((r) => r.kind === 'first-turn-divider')).toBe(false)
  })

  it('drops a suppressed message and its dividers entirely', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      eventsBefore: { 2: [{ kind: 'compaction', trigger: 'manual' }, { kind: 'interrupted' }] },
      firstTurnSequence: 2,
      suppressSequence: 2,
    })

    expect(rows.map((r) => r.kind)).toEqual(['message', 'message'])
    expect(rows.map((r) => (r.kind === 'message' ? r.message.sequence : undefined))).toEqual([1, 3])
  })

  it('gives every row a unique key', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: { 2: [{ kind: 'compaction', trigger: 'manual' }, { kind: 'interrupted' }] },
      firstTurnSequence: 1,
    })

    const keys = rows.map((r) => r.key)
    expect(new Set(keys).size).toBe(keys.length)
  })
})
