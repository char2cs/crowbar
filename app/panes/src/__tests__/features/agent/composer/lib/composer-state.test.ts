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

  it('says why the chat stopped following its ledger ahead of everything else', () => {
    const state = resolveComposerState(
      inputs({
        feedError: 'This chat’s history did not line up',
        asks: [askItem()],
      }),
    )
    expect(state).toEqual({
      kind: 'signpost',
      reason: 'unfollowed',
      message: 'This chat’s history did not line up',
    })
  })

  it('is a signpost with no runner, whatever else is true', () => {
    const state = resolveComposerState(inputs({ live: false, asks: [askItem()] }))
    expect(state).toMatchObject({ kind: 'signpost', reason: 'dormant' })
  })

  it('is a read-only signpost while the host is unreachable, over everything else', () => {
    const state = resolveComposerState(
      inputs({ offline: true, feedError: 'did not line up', asks: [askItem()] }),
    )
    expect(state).toEqual({ kind: 'signpost', reason: 'offline', message: 'Offline — read-only' })
    expect(acceptsTyping(state)).toBe(false)
  })

  it('is a signpost for a provider that cannot take a typed prompt', () => {
    expect(resolveComposerState(inputs({ submitUnavailable: true }))).toMatchObject({
      kind: 'signpost',
      reason: 'unsupported',
    })
  })

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

  it('still surfaces an ask this provider cannot receive an answer for', () => {
    const asks = [askItem({ answerable: false })]
    expect(resolveComposerState(inputs({ asks }))).toMatchObject({ kind: 'choices' })
  })

  it('is compacting when that is all that is true', () => {
    expect(resolveComposerState(inputs({ compacting: true }))).toEqual({ kind: 'compacting' })
  })
})

describe('acceptsTyping', () => {
  it('accepts typing while compacting', () => {
    expect(acceptsTyping({ kind: 'compacting' })).toBe(true)
    expect(acceptsTyping({ kind: 'input' })).toBe(true)
  })

  it('refuses typing for everything that is not an input', () => {
    expect(acceptsTyping({ kind: 'choices', asks: [askItem()] })).toBe(false)
    expect(acceptsTyping({ kind: 'signpost', reason: 'dormant', message: 'x' })).toBe(false)
  })
})
