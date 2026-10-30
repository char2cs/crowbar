import { describe, expect, it } from 'vitest'

import { askItem } from '@/__tests__/__fixtures__/ledger-rows'
import {
  acceptsTyping,
  resolveComposerState,
  type ComposerInputs,
} from '@/features/agent/composer/lib/composer-state'

function inputs(overrides: Partial<ComposerInputs> = {}): ComposerInputs {
  return {
    live: true,
    submitUnavailable: false,
    compacting: false,
    asks: [],
    ...overrides,
  }
}

describe('resolveComposerState', () => {
  it('is an input when there is nothing in the way', () => {
    expect(resolveComposerState(inputs())).toEqual({ kind: 'input' })
  })

  it('is a signpost with no runner, whatever else is true', () => {
    const state = resolveComposerState(inputs({ live: false, asks: [askItem()] }))
    expect(state).toMatchObject({ kind: 'signpost', reason: 'dormant' })
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
        asks: [askItem()],
      }),
    )
    expect(state).toMatchObject({ kind: 'signpost', reason: 'terminal_wait' })
    expect(state).toMatchObject({ message: expect.stringMatching(/trust the workspace/i) })
  })

  it('does not guess at a wait kind it has never heard of', () => {
    const state = resolveComposerState(inputs({ terminalWait: { kind: 'something_new' } }))
    expect(state).toMatchObject({ message: expect.stringMatching(/only its terminal can give/i) })
  })

  it('hands over every pending ask, in the order the fold keeps them', () => {
    const asks = [askItem({ id: 'old' }), askItem({ id: 'new' })]
    expect(resolveComposerState(inputs({ asks }))).toEqual({ kind: 'choices', asks })
  })

  // Hiding an unanswerable prompt is what made a blocked agent look frozen.
  it('still surfaces an ask this provider cannot receive an answer for', () => {
    const asks = [askItem({ answerable: false })]
    expect(resolveComposerState(inputs({ asks }))).toMatchObject({ kind: 'choices' })
  })

  it('puts a pending choice ahead of a halt', () => {
    const state = resolveComposerState(
      inputs({ asks: [askItem()], haltedMessage: 'limit reached' }),
    )
    expect(state.kind).toBe('choices')
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
