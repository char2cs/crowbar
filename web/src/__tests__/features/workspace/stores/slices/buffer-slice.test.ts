import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createBufferSlice, type BufferSlice } from '@/features/workspace/stores/slices/buffer-slice'

const makePaneActions = () => ({
  addBufferToPane: vi.fn(),
  setPanePreviewBuffer: vi.fn(),
  removeBufferFromPane: vi.fn(),
  clearPreviewBufferEverywhere: vi.fn(),
  getPaneById: vi.fn(() => null as any),
})

type PaneActions = ReturnType<typeof makePaneActions>

function makeStore(paneActions: PaneActions = makePaneActions()) {
  const store = createStore<BufferSlice & { paneActions: PaneActions }>()(
    immer((set, get) => ({
      ...createBufferSlice(set as any, get as any, {} as any),
      paneActions,
    })),
  )
  return { store, paneActions }
}

describe('buffer-slice', () => {
  let store: ReturnType<typeof makeStore>['store']
  beforeEach(() => { store = makeStore().store })

  it('starts empty', () => {
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('openContent creates an editor buffer and returns its id', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/src/index.ts', name: 'index.ts', content: 'const x = 1',
    })
    expect(id).toBeTruthy()
    expect(store.getState().buffers).toHaveLength(1)
    const buf = store.getState().buffers[0]
    expect(buf.type).toBe('editor')
    expect(buf.path).toBe('/src/index.ts')
    expect(buf.id).toBe(id)
  })

  it('openContent with the same path returns the existing buffer id', () => {
    const spec = { type: 'editor' as const, path: '/src/index.ts', name: 'index.ts', content: '' }
    const id1 = store.getState().bufferActions.openContent(spec)
    const id2 = store.getState().bufferActions.openContent(spec)
    expect(id1).toBe(id2)
    expect(store.getState().buffers).toHaveLength(1)
  })

  it('openContent creates a crowbarChat buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'crowbarChat', wsId: 'ws-1', name: 'Chat',
    })
    expect(id).toBeTruthy()
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.type).toBe('crowbarChat')
  })

  it('closeBuffer removes it from the list', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('preview flag is set when isPreview is true', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/b.ts', name: 'b.ts', content: '', isPreview: true,
    })
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.isPreview).toBe(true)
  })

  it('pin toggles isPinned on the buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/c.ts', name: 'c.ts', content: '',
    })
    store.getState().bufferActions.setPinned(id, true)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(true)
    store.getState().bufferActions.setPinned(id, false)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(false)
  })

  describe('registerExternalBuffer', () => {
    it('creates a new buffer with isPreview=false', () => {
      const { store } = makeStore()
      store.getState().bufferActions.registerExternalBuffer('b1', '/a.ts', 'a.ts', false)
      expect(store.getState().bufferActions.getBufferById('b1')?.isPreview).toBe(false)
    })

    it('creates a new buffer with isPreview=true, sets preview flag', () => {
      const paneActions = makePaneActions()
      const { store } = makeStore(paneActions)
      store.getState().bufferActions.registerExternalBuffer('b1', '/a.ts', 'a.ts', true)
      const buf = store.getState().bufferActions.getBufferById('b1')
      expect(buf?.isPreview).toBe(true)
      expect(paneActions.setPanePreviewBuffer).toHaveBeenCalled()
    })

    it('with isPreview=true: closes old preview before opening new one', () => {
      const paneActions = makePaneActions()
      // Simulate pane with existing preview
      paneActions.getPaneById.mockReturnValue({ id: 'pane-1', previewBufferId: 'old-buf' } as any)
      const { store } = makeStore(paneActions)

      // Seed the old preview buffer
      store.getState().bufferActions.registerExternalBuffer('old-buf', '/old.ts', 'old.ts', false)
      const countBefore = store.getState().buffers.length

      // Open a new preview
      store.getState().bufferActions.registerExternalBuffer('new-buf', '/new.ts', 'new.ts', true)

      expect(store.getState().bufferActions.getBufferById('old-buf')).toBeUndefined()
      expect(paneActions.removeBufferFromPane).toHaveBeenCalled()
      expect(store.getState().buffers.length).toBe(countBefore) // net 0: 1 removed, 1 added
    })

    it('does NOT close old preview when re-registering the same buffer as preview', () => {
      const paneActions = makePaneActions()
      paneActions.getPaneById.mockReturnValue({ id: 'pane-1', previewBufferId: 'same-buf' } as any)
      const { store } = makeStore(paneActions)

      store.getState().bufferActions.registerExternalBuffer('same-buf', '/a.ts', 'a.ts', true)
      const countAfterFirst = store.getState().buffers.length
      paneActions.removeBufferFromPane.mockClear()

      store.getState().bufferActions.registerExternalBuffer('same-buf', '/a.ts', 'a.ts', true)

      expect(store.getState().buffers.length).toBe(countAfterFirst)
      expect(paneActions.removeBufferFromPane).not.toHaveBeenCalled()
    })

    it('existing preview buffer: re-registering with isPreview=false clears preview', () => {
      const paneActions = makePaneActions()
      const { store } = makeStore(paneActions)

      store.getState().bufferActions.registerExternalBuffer('buf-x', '/x.ts', 'x.ts', true)
      expect(store.getState().bufferActions.getBufferById('buf-x')?.isPreview).toBe(true)

      store.getState().bufferActions.registerExternalBuffer('buf-x', '/x.ts', 'x.ts', false)
      expect(store.getState().bufferActions.getBufferById('buf-x')?.isPreview).toBe(false)
      expect(paneActions.clearPreviewBufferEverywhere).toHaveBeenCalledWith('buf-x')
    })

    it('existing permanent buffer: NOT downgraded to preview on single-click', () => {
      const paneActions = makePaneActions()
      const { store } = makeStore(paneActions)

      // Register as permanent
      store.getState().bufferActions.registerExternalBuffer('perm', '/perm.ts', 'perm.ts', false)
      paneActions.setPanePreviewBuffer.mockClear()

      // Try to open as preview — should be blocked (it's already permanent)
      store.getState().bufferActions.registerExternalBuffer('perm', '/perm.ts', 'perm.ts', true)

      expect(store.getState().bufferActions.getBufferById('perm')?.isPreview).toBe(false)
      expect(paneActions.setPanePreviewBuffer).not.toHaveBeenCalled()
    })
  })

  describe('promotePreview', () => {
    it('sets isPreview to false', () => {
      const { store: storeInst } = makeStore()
      const id = storeInst.getState().bufferActions.openContent({
        type: 'editor', path: '/src/a.ts', name: 'a.ts', content: '', isPreview: true,
      })
      storeInst.getState().bufferActions.promotePreview(id)
      expect(storeInst.getState().bufferActions.getBufferById(id)?.isPreview).toBe(false)
    })

    it('calls clearPreviewBufferEverywhere with the buffer id', () => {
      const paneActions = makePaneActions()
      const { store: storeInst } = makeStore(paneActions)
      const id = storeInst.getState().bufferActions.openContent({
        type: 'editor', path: '/src/a.ts', name: 'a.ts', content: '', isPreview: true,
      })
      storeInst.getState().bufferActions.promotePreview(id)
      expect(paneActions.clearPreviewBufferEverywhere).toHaveBeenCalledWith(id)
    })

    it('does nothing when buffer id is not found', () => {
      const paneActions = makePaneActions()
      const { store: storeInst } = makeStore(paneActions)
      // no buffer in store
      storeInst.getState().bufferActions.promotePreview('nonexistent-id')
      expect(paneActions.clearPreviewBufferEverywhere).not.toHaveBeenCalled()
    })
  })
})
