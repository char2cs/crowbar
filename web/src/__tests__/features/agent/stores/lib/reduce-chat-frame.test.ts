import { describe, expect, it } from 'vitest'

import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import {
  reduceChatFrame,
  runnerLeft,
  startedWorking,
  type ChatEntities,
  type ChatFrame,
} from '@/features/agent/stores/lib/reduce-chat-frame'

const snap = (version: number, over: Partial<AgentChat> = {}): AgentChat => ({
  id: 'c1',
  workspaceId: 'w1',
  title: 'c1',
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  working: false,
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  version,
  phase: 'dormant',
  ...over,
})

const frame = (kind: string, chat?: AgentChat, extra: Partial<ChatFrame> = {}): ChatFrame => ({
  chatId: 'c1',
  kind,
  chat,
  version: chat?.version,
  ...extra,
})

const hold = (...chats: AgentChat[]): ChatEntities => ({
  chats: Object.fromEntries(chats.map((c) => [c.id, c])),
  byWorkspace: chats.length ? { w1: chats.map((c) => c.id) } : {},
})

// Invariant A6 as a table: every row is (held, frame) → (result, outcome).
describe('reduceChatFrame', () => {
  const cases: Array<{
    name: string
    held: AgentChat[]
    frame: ChatFrame
    outcome: string
    result: Array<Pick<AgentChat, 'id' | 'version'>>
  }> = [
    {
      name: 'a chat nobody holds is taken whatever its version',
      held: [],
      frame: frame('created', snap(1)),
      outcome: 'applied',
      result: [{ id: 'c1', version: 1 }],
    },
    {
      name: 'a newer version replaces the held one',
      held: [snap(1)],
      frame: frame('turn_started', snap(2, { working: true })),
      outcome: 'applied',
      result: [{ id: 'c1', version: 2 }],
    },
    {
      name: 'an older version is dropped — an overtaken read cannot win',
      held: [snap(5)],
      frame: frame('started', snap(4, { liveRunnerId: 'stale' })),
      outcome: 'stale',
      result: [{ id: 'c1', version: 5 }],
    },
    {
      name: 'an equal version is dropped — the held answer is as fresh',
      held: [snap(5)],
      frame: frame('snapshot', snap(5, { title: 'same version' })),
      outcome: 'stale',
      result: [{ id: 'c1', version: 5 }],
    },
    {
      name: 'a delete removes the chat',
      held: [snap(5)],
      frame: frame('deleted', undefined, { version: 6 }),
      outcome: 'deleted',
      result: [],
    },
    {
      name: 'a delete older than the held snapshot is dropped',
      held: [snap(7)],
      frame: frame('deleted', undefined, { version: 6 }),
      outcome: 'stale',
      result: [{ id: 'c1', version: 7 }],
    },
    {
      name: 'a frame carrying no snapshot changes nothing',
      held: [snap(3)],
      frame: frame('placement_set'),
      outcome: 'none',
      result: [{ id: 'c1', version: 3 }],
    },
  ]

  it.each(cases)('$name', ({ held, frame: f, outcome, result }) => {
    const out = reduceChatFrame(hold(...held), f)
    expect(out.outcome.kind).toBe(outcome)
    const chats = Object.values(out.next.chats)
    expect(chats.map((c) => ({ id: c.id, version: c.version }))).toEqual(result)
    expect(out.next.byWorkspace).toEqual(result.length ? { w1: ['c1'] } : {})
  })

  it('returns the SAME state when nothing changed, so selectors do not churn', () => {
    const held = hold(snap(5))
    expect(reduceChatFrame(held, frame('snapshot', snap(4))).next).toBe(held)
  })

  it('leaves every other chat and the index untouched', () => {
    const other = { ...snap(9), id: 'c2' }
    const held = hold(snap(1), other)
    const out = reduceChatFrame(held, frame('turn_started', snap(2)))
    expect(out.next.chats.c2).toBe(other)
    expect(out.next.byWorkspace).toBe(held.byWorkspace)
  })

  it('moves a chat between workspace indexes when its workspace changes', () => {
    const out = reduceChatFrame(hold(snap(1)), frame('snapshot', snap(2, { workspaceId: 'w2' })))
    expect(out.next.byWorkspace).toEqual({ w2: ['c1'] })
  })
})

describe('edges an applied snapshot implies', () => {
  const apply = (held: AgentChat | undefined, next: AgentChat) =>
    reduceChatFrame(held ? hold(held) : hold(), frame('snapshot', next)).outcome

  it('startedWorking is the rising edge only', () => {
    expect(startedWorking(apply(snap(1), snap(2, { working: true })))).toBe(true)
    expect(startedWorking(apply(snap(1, { working: true }), snap(2, { working: true })))).toBe(
      false,
    )
    expect(startedWorking(apply(undefined, snap(1, { working: true })))).toBe(true)
    expect(startedWorking(apply(snap(3), snap(2, { working: true })))).toBe(false)
  })

  it('runnerLeft names the runner a snapshot took off the chat', () => {
    expect(runnerLeft(apply(snap(1, { liveRunnerId: 'r1' }), snap(2)))).toBe('r1')
    expect(
      runnerLeft(apply(snap(1, { liveRunnerId: 'r1' }), snap(2, { liveRunnerId: 'r2' }))),
    ).toBe('r1')
    expect(
      runnerLeft(apply(snap(1, { liveRunnerId: 'r1' }), snap(2, { liveRunnerId: 'r1' }))),
    ).toBeNull()
    expect(runnerLeft(apply(undefined, snap(1)))).toBeNull()
  })
})
