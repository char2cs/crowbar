import { beforeEach, describe, expect, it } from 'vitest'
import {
  WARM_FRESHNESS_WINDOW_MS,
  __resetActivationFreshnessForTests,
  clearWorkspaceFreshness,
  isWarmDataFresh,
  markWorkspaceDeactivated,
  peekGitFrame,
  saveGitFrame,
} from '@/features/workspace/lib/activation-freshness'

beforeEach(() => {
  __resetActivationFreshnessForTests()
})

describe('activation-freshness ledger', () => {
  it('a workspace never deactivated is not fresh (cold mount re-seeds)', () => {
    expect(isWarmDataFresh('ws-A')).toBe(false)
  })

  it('a workspace hidden within the window is fresh', () => {
    const t0 = 1_000_000
    markWorkspaceDeactivated('ws-A', t0)
    expect(isWarmDataFresh('ws-A', t0 + WARM_FRESHNESS_WINDOW_MS)).toBe(true)
    // The boundary is inclusive; one ms past it is stale.
    expect(isWarmDataFresh('ws-A', t0 + WARM_FRESHNESS_WINDOW_MS + 1)).toBe(false)
  })

  it('does NOT consume the stamp — file + git seed effects must agree on one activation', () => {
    const t0 = 2_000_000
    markWorkspaceDeactivated('ws-A', t0)
    expect(isWarmDataFresh('ws-A', t0 + 10)).toBe(true)
    // A second read (the git effect) sees the same answer as the first (the file effect).
    expect(isWarmDataFresh('ws-A', t0 + 10)).toBe(true)
  })

  it('re-stamping on a later hide extends freshness from the new time', () => {
    markWorkspaceDeactivated('ws-A', 0)
    // Long-idle: stale from the first hide.
    expect(isWarmDataFresh('ws-A', WARM_FRESHNESS_WINDOW_MS + 100)).toBe(false)
    // Hidden again just now → fresh again.
    const later = 10_000_000
    markWorkspaceDeactivated('ws-A', later)
    expect(isWarmDataFresh('ws-A', later + 5)).toBe(true)
  })

  it('tracks freshness per workspace independently', () => {
    const t0 = 3_000_000
    markWorkspaceDeactivated('ws-A', t0)
    expect(isWarmDataFresh('ws-A', t0 + 100)).toBe(true)
    expect(isWarmDataFresh('ws-B', t0 + 100)).toBe(false)
  })

  it('clearWorkspaceFreshness drops the stamp (destroyed/evicted ws re-seeds)', () => {
    const t0 = 4_000_000
    markWorkspaceDeactivated('ws-A', t0)
    clearWorkspaceFreshness('ws-A')
    expect(isWarmDataFresh('ws-A', t0 + 10)).toBe(false)
  })

  it('preserves and peeks the last git frame per workspace', () => {
    const frame = { branch: 'main', files: [{ path: 'a.ts' }] }
    expect(peekGitFrame('ws-A')).toBeNull()
    saveGitFrame('ws-A', frame)
    expect(peekGitFrame('ws-A')).toBe(frame)
    // A peek, not a pop — repeated reads (StrictMode double-mount) must agree.
    expect(peekGitFrame('ws-A')).toBe(frame)
    clearWorkspaceFreshness('ws-A')
    expect(peekGitFrame('ws-A')).toBeNull()
  })
})
