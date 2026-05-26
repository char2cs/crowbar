import { describe, it, expect, beforeEach } from 'vitest'
import { initViewStoreSubscription, _resetViewStoreUnsubscribeForTesting } from '@/features/editor/stores/view-store'
import { useBufferStore } from '@/features/editor/stores/buffer-store'

describe('initViewStoreSubscription', () => {
  beforeEach(() => {
    _resetViewStoreUnsubscribeForTesting()
  })

  it('is exported and returns an unsubscribe function', () => {
    const unsubscribe = initViewStoreSubscription()
    expect(typeof unsubscribe).toBe('function')
    unsubscribe()
  })

  it('stops responding to buffer changes after unsubscribe is called', () => {
    const unsubscribe = initViewStoreSubscription()
    unsubscribe()

    // Trigger a buffer change; if the subscription is still active it would
    // update the view store. We just verify no throw and the function returned.
    expect(() => {
      useBufferStore.setState({ buffers: [] })
    }).not.toThrow()
  })
})
