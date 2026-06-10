import type { Tree } from 'web-tree-sitter'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'
import {
  initTreeCacheSubscription,
  useTreeCacheStore,
  _resetTreeCacheSubscriptionForTesting,
} from '@/features/editor/stores/tree-cache-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

describe('initTreeCacheSubscription', () => {
  beforeEach(() => {
    _resetTreeCacheSubscriptionForTesting()
    setActiveWorkspaceStoreRef(null)
    useTreeCacheStore.setState({ trees: new Map() })
  })

  it('clears the tree cache entry when a buffer is closed', async () => {
    const store = createWorkspaceStore('test-ws')
    setActiveWorkspaceStoreRef(store)
    initTreeCacheSubscription()

    const bufferId = 'test-buffer-1'

    // Plant a fake buffer via workspace store buffers field directly
    store.setState((state) => ({
      ...state,
      buffers: [{ id: bufferId, type: 'editor', path: '/test.ts', name: 'test.ts' } as PaneContent],
    }))

    useTreeCacheStore.setState((state) => {
      const newMap = new Map(state.trees)
      newMap.set(bufferId, {
        tree: { delete: vi.fn() } as unknown as Tree,
        contentLength: 100,
        languageId: 'typescript',
        lastUpdated: Date.now(),
      })
      return { trees: newMap }
    })
    expect(useTreeCacheStore.getState().trees.has(bufferId)).toBe(true)

    // Close the buffer by removing it from the workspace store
    store.setState((state) => ({
      ...state,
      buffers: state.buffers.filter((b) => b.id !== bufferId),
    }))

    expect(useTreeCacheStore.getState().trees.has(bufferId)).toBe(false)
  })
})
