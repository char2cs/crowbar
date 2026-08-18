import { describe, expect, it } from 'vitest'

import type {
  AgentActivity,
  AgentChoice,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'
import {
  blockedOn,
  choiceDetail,
  choiceQuestions,
  choiceToolTarget,
  describeChoice,
  describeInterruption,
  describeTool,
  formatDuration,
  NO_ACTIVITY,
  pendingChoices,
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

describe('pendingChoices', () => {
  it('reports nothing when the chat is waiting on no one', () => {
    expect(pendingChoices(NO_ACTIVITY)).toEqual([])
  })

  // A prompt stops pending three ways — answered here, answered at the terminal
  // (`proceeded`), or cleared with the turn (`abandoned`) — and every one of them
  // must take it off this surface. The server's `pending` is the only authority.
  it('drops a prompt resolved ANY way, including one answered at the terminal', () => {
    const done = activity({
      choices: [
        choice({ id: 'a', pending: false, resolution: 'answered' }),
        choice({ id: 'b', seq: 2, pending: false, resolution: 'proceeded' }),
        choice({ id: 'c', seq: 3, pending: false, resolution: 'abandoned' }),
      ],
    })

    expect(pendingChoices(done)).toEqual([])
  })

  // Unlike an interruption these are NOT collapsed to the newest: each is holding
  // its own CLI gate open, and hiding one leaves the provider blocked on a
  // question nobody was shown.
  it('keeps EVERY pending prompt, oldest first', () => {
    const open = activity({
      choices: [
        choice({ id: 'b', seq: 2 }),
        choice({ id: 'a', seq: 1 }),
        choice({ id: 'gone', seq: 3, pending: false }),
      ],
    })

    expect(pendingChoices(open).map((c) => c.id)).toEqual(['a', 'b'])
  })

  // Both facts travel, and a prompt nobody can answer is still pending: the CLI
  // is genuinely asking it, at its own terminal.
  it('keeps a pending prompt that is no longer answerable', () => {
    const stale = activity({ choices: [choice({ answerable: false })] })

    expect(pendingChoices(stale)).toHaveLength(1)
  })
})

describe('choiceToolTarget', () => {
  // A prompt carries the tool's NAME; the target is on the call it gates.
  it('reads the gated call target out of the same turn', () => {
    const gated = activity({
      toolCalls: [tool({ name: 'Bash', target: 'go test ./...' })],
      choices: [choice()],
    })

    expect(choiceToolTarget(gated, choice())).toBe('go test ./...')
  })

  it('says nothing rather than guessing when no such call is in flight', () => {
    const other = activity({
      toolCalls: [
        tool({ id: 'x', name: 'Bash', turnId: 'turn-2', target: 'other turn' }),
        tool({ id: 'y', name: 'Edit', target: 'other tool' }),
        tool({ id: 'z', name: 'Bash', status: 'ok', target: 'already finished' }),
      ],
      choices: [choice()],
    })

    expect(choiceToolTarget(other, choice())).toBe('')
    expect(choiceToolTarget(NO_ACTIVITY, choice({ toolName: '' }))).toBe('')
  })
})

describe('describeChoice', () => {
  it('names the tool a permission gates', () => {
    expect(describeChoice(choice())).toBe('Run Bash?')
    expect(describeChoice(choice({ toolName: '' }))).toBe('The agent is asking for permission')
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

describe('choiceDetail', () => {
  it('puts the permission’s target under the headline', () => {
    const gated = activity({
      toolCalls: [tool({ name: 'Bash', target: 'rm -rf build' })],
      choices: [choice()],
    })

    expect(choiceDetail(gated, choice())).toBe('rm -rf build')
  })

  it('never repeats the headline it already drew', () => {
    const asked = choice({ kind: 'question', question: 'Same', title: 'Same', options: [] })

    expect(choiceDetail(NO_ACTIVITY, asked)).toBe('')
    expect(choiceDetail(NO_ACTIVITY, { ...asked, title: 'Deploy target' })).toBe('Deploy target')
  })
})
