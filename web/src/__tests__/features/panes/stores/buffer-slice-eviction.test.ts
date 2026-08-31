import { describe, it, expect } from 'vitest'
import { createWindowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

describe('buffer slice auto-eviction', () => {
  it('evicts least-recent non-protected buffer when at maxOpenTabs', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/b.ts',
      name: 'b.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    expect(store.getState().buffers).toHaveLength(2)

    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/c.ts',
      name: 'c.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    // /a.ts should be evicted (was first, least recent)
    expect(store.getState().buffers).toHaveLength(2)
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('a.ts')
    expect(store.getState().buffers.map((b) => b.name)).toContain('c.ts')
    // The evictee's tab must also be gone from the pane that held it — via
    // pane-slice's real removeEditorTabFromPane, not the old (deleted)
    // removeBufferFromPane/bufferIds shape.
    expect(store.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(idA)
  })

  it('never evicts pinned buffers', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    store.getState().bufferActions.setPinned(idA, true)
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/b.ts',
      name: 'b.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/c.ts',
      name: 'c.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    // b.ts should be evicted; a.ts (pinned) stays
    expect(store.getState().buffers.map((b) => b.name)).toContain('a.ts')
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('b.ts')
  })

  it('never evicts terminal or agent buffers', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    store.getState().bufferActions.openContent({ type: 'terminal', workspaceId: 'test-ws' })
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/b.ts',
      name: 'b.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    // Terminal stays, a.ts evicted
    expect(store.getState().buffers.map((b) => b.type)).toContain('terminal')
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('a.ts')
  })

  it('proceeds with buffer creation when no evictable buffer exists (all protected or pinned)', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 1 })

    // Terminal is protected — cannot be evicted
    store.getState().bufferActions.openContent({ type: 'terminal', workspaceId: 'test-ws' })
    expect(store.getState().buffers).toHaveLength(1)

    // Opening a new buffer proceeds even though we're at the limit (nothing evictable)
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
      workspaceId: 'test-ws',
    })
    // Terminal stays (protected), editor is also created — intentionally exceeds limit
    expect(store.getState().buffers).toHaveLength(2)
    expect(store.getState().buffers.map((b) => b.type)).toContain('terminal')
    expect(store.getState().buffers.map((b) => b.name)).toContain('a.ts')
  })
})
