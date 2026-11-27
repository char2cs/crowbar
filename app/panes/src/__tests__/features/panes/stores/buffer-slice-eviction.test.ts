import { describe, it, expect } from 'vitest'
import { createWindowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { workspaceRef } from '@/lib/api/documents'

describe('buffer slice auto-eviction', () => {
  it('evicts least-recent non-protected buffer when at maxOpenTabs', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/a.ts'),
      name: 'a.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/b.ts'),
      name: 'b.ts',
    })
    expect(store.getState().buffers).toHaveLength(2)

    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/c.ts'),
      name: 'c.ts',
    })
    expect(store.getState().buffers).toHaveLength(2)
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('a.ts')
    expect(store.getState().buffers.map((b) => b.name)).toContain('c.ts')
    expect(store.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(idA)
  })

  it('never evicts pinned buffers', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/a.ts'),
      name: 'a.ts',
    })
    store.getState().bufferActions.setPinned(idA, true)
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/b.ts'),
      name: 'b.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/c.ts'),
      name: 'c.ts',
    })
    expect(store.getState().buffers.map((b) => b.name)).toContain('a.ts')
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('b.ts')
  })

  it('never evicts terminal or agent buffers', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    store.getState().bufferActions.openContent({ type: 'terminal', workspaceId: 'test-ws' })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/a.ts'),
      name: 'a.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/b.ts'),
      name: 'b.ts',
    })
    expect(store.getState().buffers.map((b) => b.type)).toContain('terminal')
    expect(store.getState().buffers.map((b) => b.name)).not.toContain('a.ts')
  })

  it('does not evict a buffer belonging to a DIFFERENT workspace at the cap', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('ws-a', '/a.ts'),
      name: 'a.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('ws-b', '/x.ts'),
      name: 'x.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('ws-a', '/b.ts'),
      name: 'b.ts',
    })

    const names = store.getState().buffers.map((b) => b.name)
    expect(names).toContain('a.ts')
    expect(names).toContain('b.ts')
    expect(names).toContain('x.ts')
  })

  it('never auto-evicts a DIRTY buffer, even when it is the only otherwise-eligible candidate', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/a.ts'),
      name: 'a.ts',
    })
    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/b.ts'),
      name: 'b.ts',
    })
    store.setState((s) => {
      s.buffers = s.buffers.map((b) =>
        b.id === idA && b.type === 'editor' ? { ...b, isDirty: true } : b,
      )
      return s
    })

    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/c.ts'),
      name: 'c.ts',
    })

    const names = store.getState().buffers.map((b) => b.name)
    expect(names).toContain('a.ts')
    expect(names).toContain('c.ts')
  })

  it('proceeds with buffer creation when no evictable buffer exists (all protected or pinned)', () => {
    const store = createWindowPaneStore()
    store.setState({ maxOpenTabs: 1 })

    store.getState().bufferActions.openContent({ type: 'terminal', workspaceId: 'test-ws' })
    expect(store.getState().buffers).toHaveLength(1)

    store.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('test-ws', '/a.ts'),
      name: 'a.ts',
    })
    expect(store.getState().buffers).toHaveLength(2)
    expect(store.getState().buffers.map((b) => b.type)).toContain('terminal')
    expect(store.getState().buffers.map((b) => b.name)).toContain('a.ts')
  })
})
