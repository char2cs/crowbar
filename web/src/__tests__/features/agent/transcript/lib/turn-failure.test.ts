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
