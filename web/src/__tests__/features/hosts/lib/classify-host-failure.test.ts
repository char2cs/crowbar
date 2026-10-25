import { describe, expect, it } from 'vitest'
import { classifyHostFailure } from '@/features/hosts/lib/classify-host-failure'
import { ApiError } from '@/lib/api/errors'

describe('classifyHostFailure', () => {
  it.each([
    ['the proxy could not reach the host', new ApiError('x', 503, 'host_unreachable'), 'down'],
    [
      'the host presented another identity',
      new ApiError('x', 502, 'host_identity_changed'),
      'needs-repair',
    ],
    // A revoked or unknown token: no retry fixes it, only pairing again.
    ['the host refused the token (401)', new ApiError('unauthorized', 401), 'needs-repair'],
    ['the host failed (500)', new ApiError('boom', 500), 'down'],
    ['the route is missing (404)', new ApiError('not found', 404), 'down'],
    ['the request was given up', new DOMException('aborted', 'AbortError'), 'down'],
    ['the transport failed', new TypeError('Load failed'), 'down'],
    ['something that is not an error', 'nope', 'down'],
  ] as const)('%s: %s', (_name, err, status) => {
    expect(classifyHostFailure(err)).toBe(status)
  })
})
