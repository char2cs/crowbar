import { describe, expect, it } from 'vitest'

import { mapChoice, type AgentChoice } from '@/features/agent/api/agent-chat-activity'

// The wire may omit any of these; the record arrives as the daemon wrote it.
const bare = { id: 'k1', turnId: 't', seq: 1, kind: 'question', at: 'x' } as AgentChoice

describe('mapChoice', () => {
  // `answerable` grounds FALSE: buttons for a gate nobody holds reach nobody.
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

  // A three-question prompt is ONE record; modelling only the first left a live
  // agent waiting on answers to questions 2 and 3.
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

  // Absent says the record predates questions: the prompt-level one stands in.
  it('leaves an absent question list absent', () => {
    expect(mapChoice(bare).questions).toBeUndefined()
  })
})
