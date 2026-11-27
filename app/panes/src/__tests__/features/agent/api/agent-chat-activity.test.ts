import { describe, expect, it } from 'vitest'

import { mapChoice, type AgentChoice } from '@/features/agent/api/agent-chat-activity'

const bare = { id: 'k1', turnId: 't', seq: 1, kind: 'question', at: 'x' } as AgentChoice

describe('mapChoice', () => {
  it('grounds an absent answerable, pending, multi and option list', () => {
    expect(mapChoice(bare)).toMatchObject({
      answerable: false,
      pending: false,
      multi: false,
      options: [],
    })
  })

  it('keeps what the daemon did say', () => {
    const choice = mapChoice({
      ...bare,
      kind: 'tool_permission',
      options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
      pending: true,
      answerable: true,
    })
    expect(choice).toMatchObject({ pending: true, answerable: true })
    expect(choice.options).toHaveLength(1)
  })

  it('carries every question of a multi-question prompt', () => {
    const choice = mapChoice({
      ...bare,
      questions: [
        { id: 'q0', text: 'Which language?', options: [] },
        { id: 'q1', text: 'Which databases?', multi: true, options: [] },
      ],
    })
    expect(choice.questions).toHaveLength(2)
    expect(choice.questions?.[1]).toMatchObject({ id: 'q1', multi: true })
  })

  it('leaves an absent question list absent', () => {
    expect(mapChoice(bare).questions).toBeUndefined()
  })
})
