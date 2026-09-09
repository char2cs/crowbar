import { beforeEach, describe, expect, it } from 'vitest'
import {
  __resetScrollPositionsForTests,
  clearScrollPosition,
  getScrollPosition,
  setScrollPosition,
} from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('transcript-scroll-positions', () => {
  beforeEach(() => {
    __resetScrollPositionsForTests()
  })

  it('returns null for a chat with no saved position', () => {
    expect(getScrollPosition('never-seen')).toBeNull()
  })

  it('writes and reads back a position for that chat only', () => {
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 240 })
    setScrollPosition('c2', { stuck: true, distanceFromBottom: 0 })

    expect(getScrollPosition('c1')).toEqual({ stuck: false, distanceFromBottom: 240 })
    expect(getScrollPosition('c2')).toEqual({ stuck: true, distanceFromBottom: 0 })
  })

  it('replaces a previous entry rather than merging it', () => {
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 240 })
    setScrollPosition('c1', { stuck: true, distanceFromBottom: 400 })

    expect(getScrollPosition('c1')).toEqual({ stuck: true, distanceFromBottom: 400 })
  })

  it('clearScrollPosition forgets the entry', () => {
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 240 })

    clearScrollPosition('c1')

    expect(getScrollPosition('c1')).toBeNull()
  })

  // Regression: this module exists specifically because the per-workspace
  // store does NOT survive a workspace switch (destroyWorkspaceStore drops it
  // from the registry wholesale) — a position saved here must still read back
  // after whatever workspace store existed at save time is gone, which a
  // workspace-scoped store could never do for itself.
  it('survives the workspace store that was live when it was written being replaced by a new one', () => {
    const before = createWorkspaceStore('w1')
    void before // stands in for "the store destroyWorkspaceStore is about to drop"
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 500 })

    // A workspace switch back: a BRAND NEW store instance for the same
    // workspace id, exactly what the registry hands back after
    // destroyWorkspaceStore + a later re-lookup.
    const after = createWorkspaceStore('w1')
    void after

    expect(getScrollPosition('c1')).toEqual({ stuck: false, distanceFromBottom: 500 })
  })

  it("removeAgentChat's own cleanup reaches this module, not just the workspace store", () => {
    const store = createWorkspaceStore('w1')
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 240 })

    store.getState().removeAgentChat('c1')

    expect(getScrollPosition('c1')).toBeNull()
  })
})
