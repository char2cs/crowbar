import { describe, expect, it } from 'vitest'

import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import type { ActivityComponent } from '@/features/agent/lib/activity-components'
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
      eventsBefore: { 2: [{ kind: 'compaction', id: 'e1', trigger: 'manual' }] },
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
      { kind: 'interrupted', id: 'e1' },
      { kind: 'provider', id: 'e2', detail: 'codex' },
      { kind: 'model', id: 'e3', detail: 'opus' },
      { kind: 'effort', id: 'e4', detail: 'high' },
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
      eventsBefore: { 2: [{ kind: 'provider', id: 'e1', detail: 'codex' }] },
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
      eventsBefore: {
        2: [
          { kind: 'compaction', id: 'e1', trigger: 'manual' },
          { kind: 'interrupted', id: 'e2' },
        ],
      },
      firstTurnSequence: 2,
      suppressSequence: 2,
    })

    expect(rows.map((r) => r.kind)).toEqual(['message', 'message'])
    expect(rows.map((r) => (r.kind === 'message' ? r.message.sequence : undefined))).toEqual([1, 3])
  })

  it('gives every row a unique key', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      eventsBefore: {
        2: [
          { kind: 'compaction', id: 'e1', trigger: 'manual' },
          { kind: 'interrupted', id: 'e2' },
        ],
      },
      firstTurnSequence: 1,
    })

    const keys = rows.map((r) => r.key)
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('merges messages and activity by their immutable creation time and keeps component keys stable on updates', () => {
    const tool: ActivityComponent = {
      id: 'tool-1',
      turnId: 't1',
      seq: 9,
      kind: 'tool_call',
      status: 'active',
      createdAt: '2026-09-01T12:00:01.000Z',
      updatedAt: '2026-09-01T12:00:01.000Z',
      payload: { name: 'Edit' },
    }
    const messages = [
      { ...msg(2, 'assistant'), at: '2026-09-01T12:00:02.000Z' },
      { ...msg(1), at: '2026-09-01T12:00:00.000Z' },
    ]
    const initial = flattenTranscriptRows({
      messages,
      components: [tool],
      firstTurnSequence: undefined,
    })
    const updated = flattenTranscriptRows({
      messages,
      components: [{ ...tool, status: 'completed', updatedAt: '2026-09-01T12:00:03.000Z' }],
      firstTurnSequence: undefined,
    })

    expect(initial.map((row) => row.key)).toEqual(['message-1', 'activity-tool-1', 'message-2'])
    expect(updated.map((row) => row.key)).toEqual(initial.map((row) => row.key))
    expect(updated[1]).toMatchObject({
      kind: 'activity',
      component: { id: 'tool-1', status: 'completed' },
    })
  })

  it('keeps a tool after the assistant message that preceded it in the same turn', () => {
    const assistant = {
      ...msg(2, 'assistant'),
      turnId: 'turn-1',
      at: '2026-09-01T12:00:01Z',
    }
    const tool: ActivityComponent = {
      id: 'tool-1',
      turnId: 'turn-1',
      seq: 3,
      kind: 'tool_call',
      status: 'completed',
      createdAt: '2026-09-01T12:00:02Z',
      updatedAt: '2026-09-01T12:00:03Z',
      payload: { name: 'Edit' },
    }
    const rows = flattenTranscriptRows({
      messages: [assistant],
      components: [tool],
      firstTurnSequence: undefined,
    })
    expect(rows.map((row) => row.key)).toEqual(['message-2', 'activity-tool-1'])
  })

  it('hides a historical completion with no observed subagent lifetime', () => {
    const orphan: ActivityComponent = {
      id: 'orphan',
      turnId: 'turn-1',
      seq: 2,
      kind: 'subagent',
      status: 'completed',
      createdAt: '2026-09-01T12:00:01Z',
      updatedAt: '2026-09-01T12:00:01Z',
      completedAt: '2026-09-01T12:00:01Z',
      payload: {},
    }
    const rows = flattenTranscriptRows({
      messages: [],
      components: [orphan],
      firstTurnSequence: undefined,
    })
    expect(rows).toEqual([])
  })

  it('merges streaming messages into the same timeline and does not duplicate a settled sequence', () => {
    const rows = flattenTranscriptRows({
      messages: [{ ...msg(1), at: '2026-09-01T12:00:00Z' }],
      streamingMessages: [
        { ...msg(1, 'assistant'), at: '2026-09-01T12:00:00Z' },
        { ...msg(2, 'assistant'), at: '2026-09-01T12:00:02Z' },
      ],
      components: [],
      firstTurnSequence: undefined,
    })

    expect(rows.filter((row) => row.kind === 'message')).toHaveLength(2)
    expect(rows.at(-1)).toMatchObject({
      kind: 'message',
      message: { sequence: 2 },
      streaming: true,
    })
  })

  it('omits a canonical child rendered inside its owner without changing other row identities', () => {
    const diff: ActivityComponent = {
      id: 'turn-1:diff',
      turnId: 'turn-1',
      parentId: 'turn-1',
      seq: 3,
      kind: 'diff',
      status: 'completed',
      createdAt: '2026-09-01T12:00:01Z',
      updatedAt: '2026-09-01T12:00:01Z',
      payload: { unifiedDiff: 'diff --git a/a b/a' },
    }
    const plan = { ...diff, id: 'turn-1:plan', kind: 'plan' as const, payload: { steps: [] } }
    const rows = flattenTranscriptRows({
      messages: [],
      components: [diff, plan],
      excludeComponentIds: new Set([diff.id]),
      firstTurnSequence: undefined,
    })

    expect(rows.map((row) => row.key)).toEqual(['activity-turn-1:plan'])
  })

  it('renders no row for a status notice, which only echoes a message or choice', () => {
    const notice: ActivityComponent = {
      id: 'i1',
      turnId: 'turn-1',
      seq: 2,
      kind: 'status_notice',
      status: 'completed',
      createdAt: '2026-09-01T12:00:01Z',
      updatedAt: '2026-09-01T12:00:01Z',
      payload: { interruption: { id: 'i1', kind: 'permission' } },
    }
    const rows = flattenTranscriptRows({
      messages: [],
      components: [notice],
      firstTurnSequence: undefined,
    })

    expect(rows).toEqual([])
  })
})
