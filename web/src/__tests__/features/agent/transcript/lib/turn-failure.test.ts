import { describe, expect, it } from 'vitest'
import { describeFailure, warningHeadline } from '@/features/agent/transcript/lib/turn-failure'

const NOW = new Date(2026, 10, 11, 16, 46)

describe('describeFailure', () => {
  it('words a failure with no kind plainly, without the provider’s text', () => {
    expect(describeFailure({ provider: 'Provider A' })).toEqual({
      kind: 'other',
      headline: 'Provider A had a problem on its side',
      hint: 'Try again. Nothing was lost.',
      tag: 'Stopped · Provider A had a problem on its side',
    })
    expect(describeFailure({}).headline).toBe('The agent had a problem on its side')
    expect(describeFailure({ kind: 'not_a_kind' }).kind).toBe('other')
  })

  it('words an overloaded provider by its name', () => {
    expect(describeFailure({ kind: 'overloaded', provider: 'Provider A' })).toMatchObject({
      kind: 'overloaded',
      headline: 'Provider A is busy',
      hint: 'Try again in a moment.',
      tag: 'Provider A was busy · this turn did not finish',
    })
  })

  it('words a sign-in by the provider’s name', () => {
    expect(describeFailure({ kind: 'auth', provider: 'Provider A' })).toEqual({
      kind: 'auth',
      headline: 'Signed out of Provider A',
      hint: 'Sign in again to continue.',
      tag: 'Stopped · signed out of Provider A',
    })
  })

  it('says when a limit lifts, with what to do meanwhile', () => {
    const said = describeFailure({
      kind: 'limit',
      resetsAt: new Date(2026, 10, 11, 19, 0).toISOString(),
      now: NOW,
    })
    expect(said.headline).toBe('Usage limit reached')
    expect(said.tag).toBe('Stopped · usage limit reached')
    expect(said.hint).toMatch(/^Resets at 7:00.PM, in 2h 14m\. Another model can keep going now\.$/)
  })

  it('has only the next step for a limit with no known reset', () => {
    expect(describeFailure({ kind: 'limit' }).hint).toBe('Another model can keep going now.')
  })
})

describe('warningHeadline', () => {
  it('splits a warning into its first sentence and the rest', () => {
    expect(
      warningHeadline('Could not find bubblewrap on PATH. Install it with your package manager.'),
    ).toEqual({
      text: 'Could not find bubblewrap on PATH.',
      rest: 'Install it with your package manager.',
    })
    expect(warningHeadline('Model changed.')).toEqual({ text: 'Model changed.' })
  })
})
