import { describe, it, expect, beforeEach } from 'vitest'
import {
  initViewStoreSubscription,
  _resetViewStoreUnsubscribeForTesting,
} from '@/features/editor/stores/view-store'
import { setActiveWorkspaceStoreForTests } from '@/features/workspace/stores/workspace-store-registry'

describe('initViewStoreSubscription', () => {
  beforeEach(() => {
    _resetViewStoreUnsubscribeForTesting()
    setActiveWorkspaceStoreForTests(null)
  })

  it('is exported and returns an unsubscribe function', () => {
    const unsubscribe = initViewStoreSubscription()
    expect(typeof unsubscribe).toBe('function')
    unsubscribe()
  })

  it('stops responding to workspace store changes after unsubscribe is called', () => {
    const unsubscribe = initViewStoreSubscription()
    unsubscribe()

    // Trigger a workspace store change after unsubscribing; should not throw.
    expect(() => {
      setActiveWorkspaceStoreForTests(null)
    }).not.toThrow()
  })
})
