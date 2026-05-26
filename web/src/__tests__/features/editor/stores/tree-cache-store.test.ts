import { describe, it, expect, beforeEach, vi } from 'vitest'
import { initTreeCacheSubscription, useTreeCacheStore, _resetTreeCacheSubscriptionForTesting } from '@/features/editor/stores/tree-cache-store'
import { useBufferStore } from '@/features/editor/stores/buffer-store'

describe('initTreeCacheSubscription', () => {
  beforeEach(() => {
    _resetTreeCacheSubscriptionForTesting()
    // Reset stores between tests
    useTreeCacheStore.setState({ trees: new Map() })
    useBufferStore.setState({ buffers: [] })
  })

  it('clears the tree cache entry when a buffer is closed', async () => {
    initTreeCacheSubscription()

    // Allow the dynamic import inside initTreeCacheSubscription to resolve and
    // register its useBufferStore.subscribe() listener before we mutate state.
    await new Promise((r) => setTimeout(r, 0))

    // Plant a fake tree in the cache
    const bufferId = 'test-buffer-1'

    // Also plant a fake buffer in the buffer store so removal is detectable
    useBufferStore.setState({
      buffers: [{ id: bufferId, type: 'editor', path: '/test.ts', name: 'test.ts' } as any],
    })

    useTreeCacheStore.setState((state) => {
      const newMap = new Map(state.trees)
      newMap.set(bufferId, {
        tree: { delete: vi.fn() } as any,
        contentLength: 100,
        languageId: 'typescript',
        lastUpdated: Date.now(),
      })
      return { trees: newMap }
    })
    expect(useTreeCacheStore.getState().trees.has(bufferId)).toBe(true)

    // Close the buffer by removing it from buffer store
    const prevBuffers = useBufferStore.getState().buffers
    useBufferStore.setState({
      buffers: prevBuffers.filter((b) => b.id !== bufferId),
    })

    expect(useTreeCacheStore.getState().trees.has(bufferId)).toBe(false)
  })
})
