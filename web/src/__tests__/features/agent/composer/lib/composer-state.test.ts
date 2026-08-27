import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentChoice } from '@/features/agent/api/agent-api'
import {
  acceptsTyping,
  resolveComposerState,
  type ComposerInputs,
} from '@/features/agent/composer/lib/composer-state'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

function inputs(overrides: Partial<ComposerInputs> = {}): ComposerInputs {
  return {
    live: true,
    submitUnavailable: false,
    compacting: false,
    activity: NO_ACTIVITY,
    ...overrides,
  }
}

describe('resolveComposerState', () => {
  it('is an input when there is nothing in the way', () => {
    expect(resolveComposerState(inputs())).toEqual({ kind: 'input' })
  })

  it('is a signpost with no runner, whatever else is true', () => {
    const state = resolveComposerState(
      inputs({ live: false, activity: activity({ choices: [choice()] }) }),
    )
    expect(state).toMatchObject({ kind: 'signpost', reason: 'dormant' })
  })

  // `revival` refines "not live" past the plain dormant text — but only while
  // not live. It must never leak into an otherwise-normal input.
  it('ignores revival while live', () => {
    const state = resolveComposerState(
      inputs({ live: true, revival: { state: 'reviving', message: 'Resuming this chat…' } }),
    )
    expect(state).toEqual({ kind: 'input' })
  })

  it('is a reviving signpost carrying the pane’s own message', () => {
    const state = resolveComposerState(
      inputs({ live: false, revival: { state: 'reviving', message: 'Starting Claude…' } }),
    )
    expect(state).toEqual({ kind: 'signpost', reason: 'reviving', message: 'Starting Claude…' })
  })

  it('is an idle signpost naming a failed revive', () => {
    const state = resolveComposerState(
      inputs({ live: false, revival: { state: 'idle', reason: 'failed' } }),
    )
    expect(state).toMatchObject({ kind: 'signpost', reason: 'idle' })
    expect(state).toMatchObject({ message: expect.stringMatching(/could not restart/i) })
  })

  it('is an idle signpost naming a clean exit, worded differently from a failure', () => {
    const state = resolveComposerState(
      inputs({ live: false, revival: { state: 'idle', reason: 'exited' } }),
    )
    expect(state).toMatchObject({ kind: 'signpost', reason: 'idle' })
    expect(state).toMatchObject({ message: expect.stringMatching(/has exited/i) })
  })

  it('is a signpost for a provider that cannot take a typed prompt', () => {
    expect(resolveComposerState(inputs({ submitUnavailable: true }))).toMatchObject({
      kind: 'signpost',
      reason: 'unsupported',
    })
  })

  // A chat blocked on a trust dialog AND holding a pending choice shows the trust
  // dialog, because that is the one a person can actually do something about.
  it('puts a terminal wait ahead of a pending choice', () => {
    const state = resolveComposerState(
      inputs({
        terminalWait: { kind: 'workspace_trust' },
        activity: activity({ choices: [choice()] }),
      }),
    )
    expect(state).toMatchObject({ kind: 'signpost', reason: 'terminal_wait' })
    expect(state).toMatchObject({ message: expect.stringMatching(/trust the workspace/i) })
  })

  it('does not guess at a wait kind it has never heard of', () => {
    const state = resolveComposerState(inputs({ terminalWait: { kind: 'something_new' } }))
    expect(state).toMatchObject({ message: expect.stringMatching(/only its terminal can give/i) })
  })

  it('shows the OLDEST pending choice, not the newest', () => {
    const state = resolveComposerState(
      inputs({
        activity: activity({
          choices: [choice({ id: 'new', seq: 9 }), choice({ id: 'old', seq: 2 })],
        }),
      }),
    )
    expect(state).toMatchObject({ kind: 'choice', choice: { id: 'old' } })
  })

  // Hiding an unanswerable prompt is what made a blocked agent look frozen.
  it('still surfaces a choice this provider cannot receive an answer for', () => {
    const state = resolveComposerState(
      inputs({ activity: activity({ choices: [choice({ answerable: false })] }) }),
    )
    expect(state).toMatchObject({ kind: 'choice', choice: { answerable: false } })
  })

  it('ignores a resolved choice', () => {
    expect(
      resolveComposerState(
        inputs({ activity: activity({ choices: [choice({ pending: false })] }) }),
      ),
    ).toEqual({ kind: 'input' })
  })

  it('puts a pending choice ahead of a halt', () => {
    const state = resolveComposerState(
      inputs({ activity: activity({ choices: [choice()] }), haltedMessage: 'limit reached' }),
    )
    expect(state.kind).toBe('choice')
  })

  it('relays the provider’s own stop reason, with the reset when there is one', () => {
    expect(
      resolveComposerState(
        inputs({
          haltedMessage: "You've hit your usage limit.",
          haltedResetsAt: '2026-08-24T19:00:00Z',
        }),
      ),
    ).toEqual({
      kind: 'halted',
      message: "You've hit your usage limit.",
      resetsAt: '2026-08-24T19:00:00Z',
    })
  })

  it('puts a halt ahead of a compaction', () => {
    expect(resolveComposerState(inputs({ compacting: true, haltedMessage: 'stopped' })).kind).toBe(
      'halted',
    )
  })

  it('is compacting when that is all that is true', () => {
    expect(resolveComposerState(inputs({ compacting: true }))).toEqual({ kind: 'compacting' })
  })
})

describe('acceptsTyping', () => {
  // Compaction QUEUES what is typed; it does not take the box away, because a
  // disabled field would lose a thought somebody is halfway through writing.
  it('accepts typing while compacting', () => {
    expect(acceptsTyping({ kind: 'compacting' })).toBe(true)
    expect(acceptsTyping({ kind: 'input' })).toBe(true)
  })

  it('refuses typing for everything that is not an input', () => {
    expect(acceptsTyping({ kind: 'halted', message: 'x' })).toBe(false)
    expect(acceptsTyping({ kind: 'signpost', reason: 'dormant', message: 'x' })).toBe(false)
  })
})
