// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { fileUri } from '@/features/editor/lib/editor-uri'

function makeStore() {
  return createStore<PaneSlice>()(
    immer((set, get) =>
      createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
    ),
  )
}

describe('pane-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('initialises with a single empty root group', () => {
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup).not.toBeNull()
    expect(rootGroup?.id).toBe(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toEqual([])
    expect(rootGroup?.activeBufferId).toBeNull()
  })

  it('splitPane returns a new pane ID and updates rootLayout to a split', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    const rootLayout = store.getState().rootLayout
    expect(rootLayout.type).toBe('split')
    const panes = store.getState().panes
    expect(Object.keys(panes)).toContain(newPaneId!)
  })

  it('addBufferToPane adds bufferId to the correct group', () => {
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toContain('buf-1')
    expect(rootGroup?.activeBufferId).toBe('buf-1')
  })

  it('removeBufferFromPane removes bufferId from the group', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1', true)
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).not.toContain('buf-1')
    expect(rootGroup?.bufferIds).toContain('buf-2')
  })

  it('closing the active tab activates the ADJACENT tab (right neighbor, else left when last)', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-3', false)
    const activeOf = () => store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId

    // Activate the MIDDLE tab, then close it -> the right neighbor activates
    // (not the first tab, which is what dropped users onto a far-away tab).
    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, 'buf-2')
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-2')
    expect(activeOf()).toBe('buf-3')

    // buf-3 is now the last + active; closing it falls back to the left neighbor.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-3')
    expect(activeOf()).toBe('buf-1')

    // Closing the only remaining tab leaves the pane empty.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')
    expect(activeOf()).toBeNull()
  })

  // A pane can hold bufferIds whose buffer no longer exists — any caller that drops
  // a buffer without going through removeBufferFromPane strands its id here (and the
  // layout is persisted, so a stranded id outlives a reload). Activating one of those
  // ghosts renders NOTHING: the pane falls back to its empty state and the user reads
  // it as "the app lost my tabs". Observed live after deleting an agent chat.
  it('never activates a tab whose buffer no longer exists', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-real', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-ghost', false)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-active', false)

    // Only buf-real and buf-active have buffers behind them; buf-ghost is stranded.
    store.setState((st) => ({
      ...st,
      buffers: [
        { id: 'buf-real', type: 'terminal' },
        { id: 'buf-active', type: 'terminal' },
      ],
    }))

    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, 'buf-active')
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-active')

    // The right neighbour is gone, so it falls left — but must SKIP the ghost.
    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buf-real')
  })

  it('getAllPaneGroups returns all leaf groups from paneRoot and bottomRoot', () => {
    const actions = store.getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const groups = actions.getAllPaneGroups()
    // 2 from the paneRoot split + 1 from bottomRoot
    expect(groups).toHaveLength(3)
    const ids = groups.map((g) => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('togglePaneFullscreen sets fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    expect(store.getState().fullscreenPaneId).toBe(ROOT_PANE_ID)
  })

  it('exitPaneFullscreen clears fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    store.getState().paneActions.exitPaneFullscreen()
    expect(store.getState().fullscreenPaneId).toBeNull()
  })
})

describe('pane-slice bottomRoot routing', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('addBufferToPane adds to bottomRoot when paneId is BOTTOM_PANE_ID', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).toContain('buf-1')
  })

  it('getAllPaneGroups includes groups from both paneRoot and bottomRoot', () => {
    const groups = store.getState().paneActions.getAllPaneGroups()
    const ids = groups.map((g) => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('getPaneById finds pane in bottomRoot', () => {
    const pane = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('activatePaneBuffer works for bottomRoot pane', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', false)
    store.getState().paneActions.activatePaneBuffer(BOTTOM_PANE_ID, 'buf-bottom')
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.activeBufferId).toBe('buf-bottom')
  })

  it('removeBufferFromPane removes buffer from bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).not.toContain('buf-1')
    expect(bottomGroup?.bufferIds).toContain('buf-2')
  })

  it('getPaneByBufferId finds buffer in bottomRoot', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', true)
    const pane = store.getState().paneActions.getPaneByBufferId('buf-bottom')
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveBufferToPane moves buffer across trees (bottomRoot -> paneRoot)', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-x', true)
    actions.moveBufferToPane('buf-x', BOTTOM_PANE_ID, ROOT_PANE_ID)
    const rootPaneGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootPaneGroup?.bufferIds).toContain('buf-x')
    const bottomPaneGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomPaneGroup?.bufferIds).not.toContain('buf-x')
  })
})

// C1 regression: removing/closing an editor buffer must release its retained
// Monaco model via the editorManager (keyed by FILE URI), so closing a tab frees
// the model and a reopen reads fresh content. Wires a fake editorManager onto the
// store object (the same place `createWorkspaceStore` Object.assign's it).
describe('pane-slice → editorManager model release (C1)', () => {
  function makeStoreWithManager() {
    const closeBuffer = vi.fn()
    // The pane slice reads `get().buffers` (path lookup) and `api.editorManager`.
    type S = PaneSlice & { buffers: Array<{ id: string; type: string; path: string }> }
    let api: { editorManager: { closeBuffer: typeof closeBuffer } }
    const store = createStore<S>()(
      immer((set, get, rawApi) => {
        api = rawApi as unknown as typeof api
        api.editorManager = { closeBuffer }
        return {
          ...createPaneSlice(
            ...([set, get, rawApi] as unknown as Parameters<typeof createPaneSlice>),
          ),
          buffers: [{ id: 'buf-ed', type: 'editor', path: '/src/a.ts' }],
        }
      }),
    )
    return { store, closeBuffer }
  }

  it('removeBufferFromPane releases the model for that pane (paneId + fileUri)', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-ed', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-ed', true)
    expect(closeBuffer).toHaveBeenCalledWith(ROOT_PANE_ID, fileUri('/src/a.ts'))
  })

  it('does not release for a pane that never held the buffer', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-ed', true)
    expect(closeBuffer).not.toHaveBeenCalled()
  })
})
