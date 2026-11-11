import { describe, expect, it } from 'vitest'
import { describeFailure, warningHeadline } from '@/features/agent/transcript/lib/turn-failure'

const NOW = new Date(2026, 10, 11, 16, 46)

describe('describeFailure', () => {
  it('words a provider problem plainly, with no code, JSON or raw sentence', () => {
    const detail =
      '{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"The \'gpt-x\' model is not supported."}}'
    expect(describeFailure({ reason: detail, detail, provider: 'Codex' })).toEqual({
      kind: 'other',
      headline: 'Codex had a problem on its side',
      hint: 'Try again. Nothing was lost.',
      tag: 'Stopped · Codex had a problem on its side',
    })
    expect(describeFailure({ reason: 'provider_error' }).headline).toBe(
      'The agent had a problem on its side',
    )
  })

  it('words an overloaded provider without its 529', () => {
    const detail = 'API Error: 529 {"type":"error","error":{"type":"overloaded_error"}}'
    const said = describeFailure({ reason: 'overloaded', detail, provider: 'Claude' })
    expect(said).toMatchObject({
      kind: 'overloaded',
      hint: 'Try again in a moment.',
      tag: 'Claude was busy · this turn did not finish',
    })
    expect(JSON.stringify(said)).not.toMatch(/529|overloaded_error/)
  })

  it('words a sign-in by the provider’s name, without the status', () => {
    const detail = 'unexpected status 401 Unauthorized: Missing bearer'
    const said = describeFailure({ reason: detail, detail, provider: 'Claude' })
    expect(said).toEqual({
      kind: 'auth',
      headline: 'Signed out of Claude',
      hint: 'Sign in again to continue.',
      tag: 'Stopped · signed out of Claude',
    })
    expect(describeFailure({ reason: 'authentication_failed' }).kind).toBe('auth')
  })

  it('says when a limit lifts, read from the provider’s words, with what to do meanwhile', () => {
    const said = describeFailure({
      reason: 'rate_limit',
      detail: '429 rate_limit_error: Claude usage limit reached. Your limit will reset at 7pm.',
      now: new Date(2026, 10, 11, 16, 46),
    })
    expect(said.headline).toBe('Usage limit reached')
    expect(said.tag).toBe('Stopped · usage limit reached')
    expect(said.hint).toMatch(/^Resets at 7:00.PM, in 2h 14m\. Another model can keep going now\.$/)
    expect(JSON.stringify(said)).not.toMatch(/429|rate_limit_error/)
  })

  it('prefers the reset the provider reported over the one in its words', () => {
    const said = describeFailure({
      reason: 'rate_limit',
      resetsAt: new Date(2026, 10, 11, 17, 16).toISOString(),
      now: NOW,
    })
    expect(said.hint).toContain('in 30m')
  })

  it('has only the next step for a limit with no known reset', () => {
    expect(describeFailure({ reason: 'x', detail: 'HTTP 429 Too Many Requests' }).hint).toBe(
      'Another model can keep going now.',
    )
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
