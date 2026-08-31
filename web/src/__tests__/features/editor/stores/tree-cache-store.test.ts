import type { Tree } from 'web-tree-sitter'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'
import {
  initTreeCacheSubscription,
  useTreeCacheStore,
  _resetTreeCacheSubscriptionForTesting,
} from '@/features/editor/stores/tree-cache-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

describe('initTreeCacheSubscription', () => {
  beforeEach(() => {
    _resetTreeCacheSubscriptionForTesting()
    // Task 26: buffers are window-level now — reset the singleton, not a
    // per-workspace store, before each test.
    resetWindowPaneStoreForTests()
    useTreeCacheStore.setState({ trees: new Map() })
  })

  it('clears the tree cache entry when a buffer is closed', async () => {
    initTreeCacheSubscription()

    const bufferId = 'test-buffer-1'

    // Plant a fake buffer via the window pane store's buffers field directly
    windowPaneStore.setState((state) => ({
      ...state,
      buffers: [
        {
          id: bufferId,
          type: 'editor',
          path: '/test.ts',
          name: 'test.ts',
          workspaceId: 'test-ws',
        } as PaneContent,
      ],
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

    // Close the buffer by removing it from the window pane store
    windowPaneStore.setState((state) => ({
      ...state,
      buffers: state.buffers.filter((b) => b.id !== bufferId),
    }))

    expect(useTreeCacheStore.getState().trees.has(bufferId)).toBe(false)
  })
})
