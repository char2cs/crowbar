import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
// Only the scope REGISTRY is faked; the URL builders themselves run for real,
// so these assertions pin the actual paths this client dials rather than a
// stand-in shape. 'w1' carries a real repo, so chatBase builds the Task-17
// repo-scoped shape; 'ws-home' has repoId '' (a project-home workspace) and
// falls back to the still-live /home mount; anything else is unrecorded scope.
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-chat-activity'

describe('agent-chat-activity', () => {
  beforeEach(() => apiFetch.mockReset())

  // ── Prompts the CLI is blocked on ────────────────────────────────
  //
  // They ride the ACTIVITY payload; there is no second poll for them.
  it('listChatActivity carries the prompts a chat is blocked on', async () => {
    apiFetch.mockResolvedValue({
      toolCalls: [],
      subagents: [],
      interruptions: [],
      choices: [
        {
          id: 'k1',
          turnId: 'turn-1',
          seq: 4,
          kind: 'tool_permission',
          toolName: 'Bash',
          options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
          pending: true,
          answerable: true,
          at: '2026-08-18T12:00:00Z',
        },
      ],
    })

    const activity = await api.listChatActivity('w1', 'c1')

    expect(activity.choices).toHaveLength(1)
    expect(activity.choices[0]).toMatchObject({ id: 'k1', pending: true, answerable: true })
  })

  // A daemon that sends neither field is one with no answer channel at all.
  // `answerable` therefore grounds FALSE — defaulting it true would draw buttons
  // that reach nobody, which is the single failure the field exists to prevent.
  it('listChatActivity grounds an absent prompt list and an absent answerable', async () => {
    apiFetch.mockResolvedValue({
      toolCalls: [],
      subagents: [],
      interruptions: [],
      choices: [{ id: 'k1', turnId: 't', seq: 1, kind: 'question', at: 'x' }],
    })

    const activity = await api.listChatActivity('w1', 'c1')

    expect(activity.choices[0]).toMatchObject({
      answerable: false,
      pending: false,
      multi: false,
      options: [],
    })

    apiFetch.mockResolvedValue({})
    expect((await api.listChatActivity('w1', 'c1')).choices).toEqual([])
  })

  // A three-question prompt is ONE record carrying three questions, and it has to
  // survive the read intact: modelling only the first is what left a live agent
  // saying "still waiting on your answers to questions 2 & 3".
  it('listChatActivity carries every question of a multi-question prompt', async () => {
    apiFetch.mockResolvedValue({
      choices: [
        {
          id: 'k1',
          turnId: 'turn-1',
          seq: 4,
          kind: 'question',
          options: [],
          questions: [
            {
              id: 'q0',
              text: 'Which language?',
              options: [{ id: 'q0-answer-0', kind: 'answer', label: 'Go' }],
            },
            {
              id: 'q1',
              text: 'Which databases?',
              multi: true,
              options: [{ id: 'q1-answer-0', kind: 'answer', label: 'SQLite' }],
            },
          ],
          pending: true,
          answerable: true,
          at: '2026-08-18T12:00:00Z',
        },
      ],
    })

    const activity = await api.listChatActivity('w1', 'c1')

    expect(activity.choices[0]?.questions).toHaveLength(2)
    expect(activity.choices[0]?.questions?.[1]).toMatchObject({ id: 'q1', multi: true })
  })

  // ABSENT is not the same as empty here: it says the record predates questions
  // being modelled, and a client falls back to the prompt-level question rather
  // than drawing a card with nothing in it. So it is never grounded to [].
  it('listChatActivity leaves an absent question list absent', async () => {
    apiFetch.mockResolvedValue({
      choices: [{ id: 'k1', turnId: 't', seq: 1, kind: 'question', at: 'x' }],
    })

    const activity = await api.listChatActivity('w1', 'c1')

    expect(activity.choices[0]?.questions).toBeUndefined()
  })
})
