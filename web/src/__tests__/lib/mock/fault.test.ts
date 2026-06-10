import { describe, it, expect } from 'vitest'
import { shouldFault } from '@/lib/mock/fault'

function makeRequest(faultHeader?: string): Request {
  const headers = new Headers()
  if (faultHeader) headers.set('X-Crowbar-Fault', faultHeader)
  return new Request('http://localhost/api/test', { headers })
}

describe('shouldFault', () => {
  it('returns false when no X-Crowbar-Fault header', () => {
    expect(shouldFault(makeRequest(), 'branch-diff')).toBe(false)
  })

  it('returns false when key not in header', () => {
    expect(shouldFault(makeRequest('{"workspaces":100}'), 'branch-diff')).toBe(false)
  })

  it('returns true when key is 100%', () => {
    const results = Array.from({ length: 20 }, () =>
      shouldFault(makeRequest('{"branch-diff":100}'), 'branch-diff'),
    )
    expect(results.every(Boolean)).toBe(true)
  })

  it('returns false when key is 0%', () => {
    const results = Array.from({ length: 20 }, () =>
      shouldFault(makeRequest('{"branch-diff":0}'), 'branch-diff'),
    )
    expect(results.every((r) => !r)).toBe(true)
  })

  it('returns false for invalid JSON', () => {
    expect(shouldFault(makeRequest('not-json'), 'branch-diff')).toBe(false)
  })
})
