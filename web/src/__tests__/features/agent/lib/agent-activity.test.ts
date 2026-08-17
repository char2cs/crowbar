import { describe, expect, it } from 'vitest'

import type {
  AgentActivity,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'
import {
  blockedOn,
  describeInterruption,
  describeTool,
  formatDuration,
  NO_ACTIVITY,
  runningSubagents,
  runningTools,
} from '@/features/agent/lib/agent-activity'

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: 't1',
    turnId: 'turn-1',
    seq: 1,
    name: 'Bash',
    status: 'running',
    hasRequest: false,
    hasResult: false,
    startedAt: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

function interruption(overrides: Partial<AgentInterruption> = {}): AgentInterruption {
  return {
    id: 'i1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'permission',
    at: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

describe('blockedOn', () => {
  it('reports nothing when every interruption is resolved', () => {
    const resolved = interruption({ resolvedAt: '2026-08-17T12:00:01Z' })

    expect(blockedOn(activity({ interruptions: [resolved] }))).toBeNull()
    expect(blockedOn(NO_ACTIVITY)).toBeNull()
  })

  // They are states, not a log. A stack of "waiting for permission" banners tells
  // a user nothing the top one does not.
  it('surfaces only the latest unresolved interruption', () => {
    const older = interruption({ id: 'i1', seq: 1 })
    const newer = interruption({ id: 'i2', seq: 9, kind: 'compaction' })
    const done = interruption({ id: 'i3', seq: 12, resolvedAt: '2026-08-17T12:00:02Z' })

    expect(blockedOn(activity({ interruptions: [older, newer, done] }))?.id).toBe('i2')
  })
})

describe('runningTools', () => {
  it('keeps only running calls, in the order they started', () => {
    const calls = [
      tool({ id: 'b', seq: 5 }),
      tool({ id: 'done', seq: 2, status: 'ok' }),
      tool({ id: 'a', seq: 3 }),
      tool({ id: 'gone', seq: 4, status: 'abandoned' }),
    ]

    expect(runningTools(activity({ toolCalls: calls })).map((c) => c.id)).toEqual(['a', 'b'])
  })
})

describe('runningSubagents', () => {
  // Starts and stops observe DIFFERENT populations on both providers, so this
  // counts what has a start and no end rather than reconciling the two.
  it('counts subagents with no recorded end', () => {
    const subagents = [
      { id: 'a', turnId: 't', seq: 1, startedAt: 'x' },
      { id: 'b', turnId: 't', seq: 2, startedAt: 'x', endedAt: 'y' },
      { id: 'c', turnId: 't', seq: 3, startedAt: 'x' },
    ]

    expect(runningSubagents(activity({ subagents }))).toBe(2)
    expect(runningSubagents(NO_ACTIVITY)).toBe(0)
  })
})

describe('describeTool', () => {
  it('names what the tool acted on when the provider reported one', () => {
    expect(describeTool(tool({ name: 'Edit', target: 'a.go' }))).toBe('Edit · a.go')
  })

  it('falls back to the tool alone rather than inventing a target', () => {
    expect(describeTool(tool({ name: 'Bash' }))).toBe('Bash')
    expect(describeTool(tool({ name: 'Bash', target: '' }))).toBe('Bash')
  })
})

describe('describeInterruption', () => {
  it('says something different for each kind, because each IS different', () => {
    expect(describeInterruption(interruption({ kind: 'permission', detail: 'Bash' }))).toContain(
      'Bash',
    )
    expect(describeInterruption(interruption({ kind: 'permission' }))).toBe(
      'Waiting for your permission',
    )
    expect(describeInterruption(interruption({ kind: 'notification', detail: 'look here' }))).toBe(
      'look here',
    )
    expect(describeInterruption(interruption({ kind: 'compaction' }))).toContain('Compacting')
    expect(describeInterruption(interruption({ kind: 'elicitation' }))).toContain('waiting')
  })
})

describe('formatDuration', () => {
  it.each([
    [undefined, ''],
    [0, ''],
    [-5, ''],
    // Sub-second work in milliseconds: "0s" reads as "nothing happened".
    [37, '37ms'],
    [1500, '1.5s'],
    [59_000, '59.0s'],
    [60_000, '1m'],
    [90_000, '1m 30s'],
  ])('renders %s as %s', (ms, expected) => {
    expect(formatDuration(ms)).toBe(expected)
  })
})
