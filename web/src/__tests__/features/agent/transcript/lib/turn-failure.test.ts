import { describe, expect, it } from 'vitest'
import { describeFailure } from '@/features/agent/transcript/lib/turn-failure'

describe('describeFailure', () => {
  // Recorded live from codex 0.167 with an unknown model.
  it('reads the message out of a provider error that arrives as JSON', () => {
    const detail =
      '{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"The \'gpt-not-a-model\' model is not supported when using Codex with a ChatGPT account.","code":"model_not_found"}}'
    expect(describeFailure(detail, detail)).toEqual({
      kind: 'other',
      text: "The 'gpt-not-a-model' model is not supported when using Codex with a ChatGPT account.",
      more: detail,
    })
  })

  // Synthetic: claude's TUI wording of an API error; no StopFailure is recorded.
  it('reads the message out of a JSON body after a prefix', () => {
    const detail =
      'API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}'
    expect(describeFailure('overloaded', detail)).toEqual({
      kind: 'other',
      text: 'Overloaded',
      more: detail,
    })
  })

  // Overloaded lifts on its own and is retried: no usage window, no sign-in.
  it('words an overloaded provider as busy, not as a usage limit', () => {
    expect(describeFailure('overloaded')).toEqual({
      kind: 'other',
      text: 'The provider is overloaded right now.',
    })
  })

  it('tells a sign-in, a limit and anything else apart', () => {
    expect(describeFailure('authentication_failed').kind).toBe('auth')
    expect(describeFailure('x', 'unexpected status 401 Unauthorized').kind).toBe('auth')
    expect(describeFailure('rate_limit')).toEqual({
      kind: 'limit',
      text: 'The provider’s usage limit was reached.',
    })
    expect(describeFailure('turn_failed', 'boom\nstack')).toEqual({
      kind: 'other',
      text: 'boom',
      more: 'stack',
    })
  })
})
