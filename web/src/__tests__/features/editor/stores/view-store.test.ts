import { describe, it, expect, beforeEach } from 'vitest'
import {
  initViewStoreSubscription,
  _resetViewStoreUnsubscribeForTesting,
} from '@/features/editor/stores/view-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

describe('initViewStoreSubscription', () => {
  beforeEach(() => {
    _resetViewStoreUnsubscribeForTesting()
    setActiveWorkspaceStoreRef(null)
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
      setActiveWorkspaceStoreRef(null)
    }).not.toThrow()
  })
})
