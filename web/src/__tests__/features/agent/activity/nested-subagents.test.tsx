import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentSubagent } from '@/features/agent/api/agent-api'
import { finishedNestedSubagents } from '@/features/agent/activity/nested-subagents'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function subagent(overrides: Partial<AgentSubagent> = {}): AgentSubagent {
  return {
    id: 'a1',
    turnId: '',
    seq: 1,
    startedAt: '2026-08-17T12:00:00Z',
    endedAt: '2026-08-17T12:00:05Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

describe('finishedNestedSubagents', () => {
  it('keeps only ended subagents with no top-level turn of their own', () => {
    const got = finishedNestedSubagents(
      activity({
        subagents: [
          subagent({ id: 'nested-done', turnId: '' }),
          subagent({ id: 'nested-running', turnId: '', endedAt: undefined }),
          subagent({ id: 'claude-native', turnId: 'turn-1' }),
        ],
      }),
    )

    expect(got.map((s) => s.id)).toEqual(['nested-done'])
  })

  it('sorts by seq', () => {
    const got = finishedNestedSubagents(
      activity({
        subagents: [
          subagent({ id: 'second', turnId: '', seq: 2 }),
          subagent({ id: 'first', turnId: '', seq: 1 }),
        ],
      }),
    )

    expect(got.map((s) => s.id)).toEqual(['first', 'second'])
  })
})
