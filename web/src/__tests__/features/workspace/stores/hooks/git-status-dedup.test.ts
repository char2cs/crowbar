import { describe, it, expect } from 'vitest'
import { framesEqual } from '@/features/workspace/stores/hooks/use-workspace-effects'

// P5d: the git/status push stream repeats identical frames far faster than
// the reload debounce (~165ms apart, indefinitely). The dedup check used to
// JSON.stringify every multi-KB frame ~6x/sec, competing with typing for main
// -thread time. framesEqual replaces that with fast-deep-equal, which walks
// the already-parsed frame object and bails on the first mismatch instead of
// allocating a fresh string every call.
describe('framesEqual', () => {
  it('treats structurally identical frames as equal', () => {
    const prev = { branch: 'main', files: [{ path: 'a.ts', status: 'M' }] }
    const next = { branch: 'main', files: [{ path: 'a.ts', status: 'M' }] }
    expect(framesEqual(prev, next)).toBe(true)
  })

  it('treats a frame with a nested change as different', () => {
    const prev = { branch: 'main', files: [{ path: 'a.ts', status: 'M' }] }
    const next = { branch: 'main', files: [{ path: 'a.ts', status: 'A' }] }
    expect(framesEqual(prev, next)).toBe(false)
  })

  // The very first frame has no predecessor: `lastFrame` starts as `null`,
  // and null must never compare equal to an incoming frame, or the first
  // status frame of a session would silently fail to trigger a reload.
  it('never treats a null previous frame as equal, so the first frame always triggers a reload', () => {
    expect(framesEqual(null, { branch: 'main', files: [] })).toBe(false)
  })
})
