import { describe, expect, it } from 'vitest'
import { describeFailure } from '@/features/agent/transcript/lib/turn-failure'

describe('describeFailure', () => {
  // Recorded live from codex 0.167 with an unknown model.
  it('reads the message out of a provider error that arrives as JSON', () => {
    const detail =
      '{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"The \'gpt-not-a-model\' model is not supported when using Codex with a ChatGPT account.","code":"model_not_found"}}'
    expect(describeFailure(detail, detail)).toEqual({
      kind: 'other',
      headline:
        "The 'gpt-not-a-model' model is not supported when using Codex with a ChatGPT account.",
      more: detail,
    })
  })

  // Synthetic: claude's TUI wording of an API error; no StopFailure is recorded.
  it('words an overloaded provider plainly, with no code or body', () => {
    const detail =
      'API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}'
    expect(describeFailure('overloaded', detail)).toEqual({
      kind: 'overloaded',
      headline: 'The provider is overloaded.',
      hint: 'Try again in a moment.',
      more: detail,
    })
    expect(describeFailure('overloaded')).toEqual({
      kind: 'overloaded',
      headline: 'The provider is overloaded.',
      hint: 'Try again in a moment.',
    })
  })

  it('words a sign-in in two short lines and keeps the raw status behind more', () => {
    const detail = 'unexpected status 401 Unauthorized: Missing bearer, cf-ray: a5678360fab4b516'
    expect(describeFailure(detail, detail)).toEqual({
      kind: 'auth',
      headline: 'You are signed out',
      hint: 'Sign in again to continue.',
      more: detail,
    })
    expect(describeFailure('authentication_failed').kind).toBe('auth')
  })

  it('words a usage limit without the provider’s sentence', () => {
    expect(describeFailure('rate_limit')).toEqual({
      kind: 'limit',
      headline: 'Usage limit reached',
    })
    expect(describeFailure('x', 'HTTP 429 Too Many Requests').headline).toBe('Usage limit reached')
  })

  it('keeps an unrecognised failure’s first line, never a bare code or number', () => {
    expect(describeFailure('turn_failed', 'boom\nstack')).toEqual({
      kind: 'other',
      headline: 'boom',
      more: 'boom\nstack',
    })
    expect(describeFailure('provider_error')).toEqual({
      kind: 'other',
      headline: 'The provider stopped this turn.',
    })
    expect(describeFailure('x', 'API Error: 500 Internal server error').headline).toBe(
      'Internal server error',
    )
  })
})
