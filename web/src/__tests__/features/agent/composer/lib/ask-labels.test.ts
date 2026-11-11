import { describe, expect, it } from 'vitest'

import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import {
  askNote,
  choiceDetail,
  choiceQuestions,
  describeChoice,
  describeResolvedChoice,
  optionLabel,
} from '@/features/agent/composer/lib/ask-labels'

function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [
      { id: 'allow', kind: 'allow', label: 'Allow' },
      { id: 'deny', kind: 'deny', label: 'Deny' },
    ],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

describe('describeChoice', () => {
  it('names the tool a permission gates', () => {
    expect(describeChoice(choice())).toBe('Run Bash')
    expect(describeChoice(choice({ toolName: '' }))).toBe('The agent is asking for permission')
  })

  // A provider's internal tool name ("commandExecution") is not words; the call's kind is.
  it('names the kind of call a permission gates when its call is known', () => {
    const run = choice({ toolName: 'commandExecution' })
    expect(describeChoice(run, 'execute')).toBe('Run a command')
    expect(describeChoice(choice({ toolName: 'fileChange' }), 'edit')).toBe('Edit a file')
    expect(describeChoice(run, 'other')).toBe('Run commandExecution')
  })

  it('asks to approve a plan when the gated call is a plan’s', () => {
    expect(describeChoice(choice({ toolName: 'ExitPlanMode' }), 'plan')).toBe('Approve this plan')
  })

  it('asks a question in the provider’s own words', () => {
    const asked = choice({ kind: 'question', question: 'Which option do you prefer?', options: [] })

    expect(describeChoice(asked)).toBe('Which option do you prefer?')
  })

  // With three questions no single one is what the prompt is asking, and naming
  // one of them would be a lie a reader could act on — so the count is said.
  it('counts the questions when there is no single one to quote', () => {
    const asked = choice({
      kind: 'question',
      question: '',
      title: '',
      options: [],
      questions: [
        { id: 'q0', text: 'a', options: [] },
        { id: 'q1', text: 'b', options: [] },
        { id: 'q2', text: 'c', options: [] },
      ],
    })

    expect(describeChoice(asked)).toBe('The agent has 3 questions')
    expect(describeChoice({ ...asked, questions: [{ id: 'q0', text: '', options: [] }] })).toBe(
      'The agent has a question',
    )
  })

  // A kind this build has never heard of falls through to whatever the provider
  // did say, never to a guess about what it meant.
  it('degrades to the provider’s own text for an unknown kind', () => {
    expect(describeChoice(choice({ kind: 'something_new', question: 'Well?' }))).toBe('Well?')
    expect(describeChoice(choice({ kind: 'something_new', question: '', title: 'T' }))).toBe('T')
  })
})

describe('choiceQuestions', () => {
  // A prompt recorded since questions were modelled carries them directly.
  it('reports every question of a multi-question prompt', () => {
    const asked = choice({
      kind: 'question',
      options: [],
      questions: [
        { id: 'q0', text: 'Which language?', options: [{ id: 'q0-answer-0', kind: 'answer' }] },
        { id: 'q1', text: 'Which database?', options: [{ id: 'q1-answer-0', kind: 'answer' }] },
      ],
    })

    expect(choiceQuestions(asked).map((q) => q.text)).toEqual([
      'Which language?',
      'Which database?',
    ])
  })

  // The graceful fallback, and NOT a migration: a row written before questions
  // existed is a single question described by the prompt's own text and options.
  it('presents a prompt recorded before questions existed as a question of one', () => {
    const legacy = choice({
      kind: 'question',
      question: 'Which do you want?',
      multi: true,
      options: [
        { id: 'answer-0', kind: 'answer', label: 'A' },
        { id: 'answer-1', kind: 'answer', label: 'B' },
      ],
    })

    expect(choiceQuestions(legacy)).toEqual([
      {
        id: 'q0',
        title: undefined,
        text: 'Which do you want?',
        multi: true,
        options: legacy.options,
      },
    ])
  })

  // A permission's allow/deny and an elicitation's verbs are not a pick from a
  // list the agent offered, so neither is a question in this sense.
  it('reports nothing for a permission or an elicitation', () => {
    expect(choiceQuestions(choice())).toEqual([])
    expect(choiceQuestions(choice({ kind: 'elicitation', options: [] }))).toEqual([])
  })
})

describe('optionLabel', () => {
  it('uses the provider’s own label when there is one', () => {
    expect(optionLabel({ id: 'allow', kind: 'allow', label: 'Allow' })).toBe('Allow')
  })

  it('title-cases the kind when the provider labels nothing', () => {
    expect(optionLabel({ id: 'allow', kind: 'allow' })).toBe('Allow')
  })
})

describe('describeResolvedChoice', () => {
  it('names what was picked, against the tool it gated', () => {
    const answered = choice({
      pending: false,
      resolution: 'answered',
      answeredOptionIds: ['allow'],
    })

    expect(describeResolvedChoice(answered)).toBe('Bash · Allow')
  })

  it('says an answer went through Crowbar even without which one, for an older record', () => {
    const answered = choice({ pending: false, resolution: 'answered' })

    expect(describeResolvedChoice(answered)).toBe('Bash · Answered')
  })

  it.each([
    ['proceeded', 'Bash · answered at the terminal'],
    ['refused', 'Bash · refused, no answer in time'],
    ['abandoned', 'Bash · left unanswered'],
  ])('tells a terminal answer, a refusal and no answer at all apart: %s', (resolution, line) => {
    expect(describeResolvedChoice(choice({ pending: false, resolution }))).toBe(line)
  })

  it('falls back to the prompt’s own headline when there is no gated tool', () => {
    const asked = choice({
      kind: 'question',
      toolName: '',
      question: 'Which do you want?',
      options: [],
      pending: false,
      resolution: 'proceeded',
    })

    expect(describeResolvedChoice(asked)).toBe('Which do you want? · answered at the terminal')
  })

  it('resolves the picks against a QUESTION’s own options too', () => {
    const answered = choice({
      kind: 'question',
      toolName: '',
      question: 'Which?',
      options: [],
      questions: [{ id: 'q0', text: 'Which?', options: [{ id: 'a', kind: 'answer', label: 'A' }] }],
      pending: false,
      resolution: 'answered',
      answeredOptionIds: ['a'],
    })

    expect(describeResolvedChoice(answered)).toMatch(/· A$/)
  })

  // An unrecognised label beats dropping the fact that something was picked.
  it('falls back to the raw id when it names no known option', () => {
    const answered = choice({ pending: false, resolution: 'answered', answeredOptionIds: ['x'] })

    expect(describeResolvedChoice(answered)).toBe('Bash · x')
  })
})

describe('choiceDetail', () => {
  it('puts the permission’s target under the headline', () => {
    expect(choiceDetail(choice(), 'rm -rf build')).toBe('rm -rf build')
  })

  it('never repeats the headline it already drew', () => {
    const asked = choice({ kind: 'question', question: 'Same', title: 'Same', options: [] })

    expect(choiceDetail(asked)).toBe('')
    expect(choiceDetail({ ...asked, title: 'Deploy target' })).toBe('Deploy target')
  })
})

describe('askNote', () => {
  it('says an allow once, not again in the option’s own word', () => {
    expect(askNote(choice({ pending: false, answeredOptionIds: ['allow'] }))).toBe('Allowed')
  })

  it('keeps an allow option’s label when it says more than allow', () => {
    const options = [
      { id: 'allow', kind: 'allow', label: 'Allow' },
      { id: 'session', kind: 'allow', label: 'Allow for this session' },
      { id: 'deny', kind: 'deny', label: 'Deny' },
    ]
    expect(askNote(choice({ options, pending: false, answeredOptionIds: ['session'] }))).toBe(
      'Allowed · Allow for this session',
    )
  })
})
