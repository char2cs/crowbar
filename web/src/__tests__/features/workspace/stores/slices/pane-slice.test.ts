// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { fileUri } from '@/features/editor/lib/editor-uri'

function makeStore() {
  return createStore<PaneSlice>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
    })),
  )
}

function makeStoreWithBuffers(buffers: Array<Record<string, unknown>>) {
  return createStore<PaneSlice & { buffers: Array<Record<string, unknown>> }>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
      buffers,
    })),
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
    expect(rootGroup?.chatId).toBeNull()
    expect(rootGroup?.runnerId).toBeNull()
    expect(rootGroup?.editorTabIds).toEqual([])
    expect(rootGroup?.activeEditorTabId).toBeNull()
    expect(rootGroup?.editorOpen).toBe(false)
  })

  it('splitPane returns a new pane ID and updates rootLayout to a split', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    const rootLayout = store.getState().rootLayout
    expect(rootLayout.type).toBe('split')
    const panes = store.getState().panes
    expect(Object.keys(panes)).toContain(newPaneId!)
    // A split with no seed tab lands on the empty stage, not a stray tab.
    const newPane = store.getState().paneActions.getPaneById(newPaneId!)
    expect(newPane?.chatId).toBeNull()
    expect(newPane?.editorTabIds).toEqual([])
    expect(newPane?.editorOpen).toBe(false)
  })

  it('splitPane seeds the new pane with the given tab and opens the editor view', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')
    const newPane = store.getState().paneActions.getPaneById(newPaneId!)
    expect(newPane?.editorTabIds).toEqual(['tab-1'])
    expect(newPane?.activeEditorTabId).toBe('tab-1')
    expect(newPane?.editorOpen).toBe(true)
  })

  it('addEditorTabToPane adds the tab to the correct group, activates it, and opens the editor view', () => {
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
    })
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).toContain('tab-1')
    expect(rootGroup?.activeEditorTabId).toBe('tab-1')
    expect(rootGroup?.editorOpen).toBe(true)
  })

  it('removeEditorTabFromPane removes the tab from the group', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-2', type: 'editor', name: 'b.ts' })
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).not.toContain('tab-1')
    expect(rootGroup?.editorTabIds).toContain('tab-2')
  })

  it('removeEditorTabFromPane closes the editor view once the last tab is gone', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    const rootGroup = actions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).toEqual([])
    expect(rootGroup?.activeEditorTabId).toBeNull()
    expect(rootGroup?.editorOpen).toBe(false)
  })

  it('closing the active tab activates the ADJACENT tab (right neighbor, else left when last)', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-2', type: 'editor', name: 'b.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-3', type: 'editor', name: 'c.ts' })
    const activeOf = () => store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId

    // Activate the MIDDLE tab, then close it -> the right neighbor activates
    // (not the first tab, which is what dropped users onto a far-away tab).
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-2')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-2')
    expect(activeOf()).toBe('tab-3')

    // tab-3 is now the last + active; closing it falls back to the left neighbor.
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-3')
    expect(activeOf()).toBe('tab-1')

    // Closing the only remaining tab leaves the pane empty.
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    expect(activeOf()).toBeNull()
  })

  // A pane can hold editorTabIds whose content no longer exists — any caller that
  // drops a buffer without going through removeEditorTabFromPane strands its id
  // here. Activating one of those ghosts renders NOTHING.
  it('never activates a tab whose content no longer exists', () => {
    const store = makeStoreWithBuffers([
      { id: 'tab-real', type: 'terminal' },
      { id: 'tab-active', type: 'terminal' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-real', type: 'terminal', name: 'sh' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-ghost', type: 'terminal', name: 'sh' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-active', type: 'terminal', name: 'sh' })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-active')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-active')

    // The right neighbour (tab-ghost) has no buffer behind it, so it falls
    // left, skipping the ghost.
    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe(
      'tab-real',
    )
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

  it('setPaneLocked sets the locked flag', () => {
    store.getState().paneActions.setPaneLocked(ROOT_PANE_ID, true)
    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.locked).toBe(true)
    store.getState().paneActions.setPaneLocked(ROOT_PANE_ID, false)
    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.locked).toBe(false)
  })

  it('getActivePane returns the pane matching activePaneId', () => {
    const actions = store.getState().paneActions
    const newPaneId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    expect(actions.getActivePane()?.id).toBe(newPaneId)
    actions.setActivePane(ROOT_PANE_ID)
    expect(actions.getActivePane()?.id).toBe(ROOT_PANE_ID)
  })
})

describe('pane-slice bottomRoot routing', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('addEditorTabToPane adds to bottomRoot when paneId is BOTTOM_PANE_ID', () => {
    store
      .getState()
      .paneActions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-1', type: 'terminal', name: 'sh' })
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.editorTabIds).toContain('tab-1')
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

  it('activateEditorTabInPane works for bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-bottom', type: 'terminal', name: 'sh' })
    actions.activateEditorTabInPane(BOTTOM_PANE_ID, 'tab-bottom')
    const bottomGroup = actions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.activeEditorTabId).toBe('tab-bottom')
  })

  it('removeEditorTabFromPane removes tab from bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-1', type: 'terminal', name: 'sh' })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-2', type: 'terminal', name: 'sh' })
    actions.removeEditorTabFromPane(BOTTOM_PANE_ID, 'tab-1')
    const bottomGroup = actions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.editorTabIds).not.toContain('tab-1')
    expect(bottomGroup?.editorTabIds).toContain('tab-2')
  })

  it('getPaneByEditorTabId finds a tab in bottomRoot', () => {
    store.getState().paneActions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-bottom',
      type: 'terminal',
      name: 'sh',
    })
    const pane = store.getState().paneActions.getPaneByEditorTabId('tab-bottom')
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveEditorTabToPane moves a tab across trees (bottomRoot -> paneRoot)', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-x', type: 'terminal', name: 'sh' })
    actions.moveEditorTabToPane('tab-x', BOTTOM_PANE_ID, ROOT_PANE_ID)
    const rootPaneGroup = actions.getPaneById(ROOT_PANE_ID)
    expect(rootPaneGroup?.editorTabIds).toContain('tab-x')
    expect(rootPaneGroup?.activeEditorTabId).toBe('tab-x')
    expect(rootPaneGroup?.editorOpen).toBe(true)
    const bottomPaneGroup = actions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomPaneGroup?.editorTabIds).not.toContain('tab-x')
    // The source pane is left empty (and its editor view collapses) rather
    // than being force-closed — that decision belongs to closePane, not a move.
    expect(bottomPaneGroup?.editorTabIds).toEqual([])
    expect(bottomPaneGroup?.editorOpen).toBe(false)
  })
})

// A pane's chat and its editor tabs are independent axes now — closing/moving
// tabs never touches chatId/runnerId, and setPaneChat never touches editorTabIds.
describe('pane-slice — setPaneChat', () => {
  it('sets exactly one chat on a pane, replacing any prior one', () => {
    const store = createWorkspaceStore('ws-test')
    const paneId = store.getState().panes[ROOT_PANE_ID]?.id ?? ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')
    expect(store.getState().paneActions.getPaneById(paneId)?.chatId).toBe('chat-1')
    store.getState().paneActions.setPaneChat(paneId, 'chat-2', 'runner-2')
    expect(store.getState().paneActions.getPaneById(paneId)?.chatId).toBe('chat-2')
    expect(store.getState().paneActions.getPaneById(paneId)?.runnerId).toBe('runner-2')
  })

  it('editor tabs are independent of the chat', () => {
    const store = createWorkspaceStore('ws-test')
    const paneId = store.getState().panes[ROOT_PANE_ID]?.id ?? ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')
    store
      .getState()
      .paneActions.addEditorTabToPane(paneId, { id: 'file-1', type: 'editor', name: 'foo.ts' })
    expect(store.getState().paneActions.getPaneById(paneId)?.editorTabIds).toContain('file-1')
    expect(store.getState().paneActions.getPaneById(paneId)?.chatId).toBe('chat-1')
  })

  it('clearing the chat leaves editorTabIds untouched', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'file-1',
      type: 'editor',
      name: 'foo.ts',
    })
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)
    const pane = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(pane?.chatId).toBeNull()
    expect(pane?.runnerId).toBeNull()
    expect(pane?.editorTabIds).toEqual(['file-1'])
  })
})

describe('pane-slice — reorderEditorTabs', () => {
  it('moves a tab to the target index', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'b', type: 'editor', name: 'b.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'c', type: 'editor', name: 'c.ts' })

    actions.reorderEditorTabs(ROOT_PANE_ID, 'a', 2)

    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['b', 'c', 'a'])
  })

  it('is a no-op when the tab is not in the pane', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.reorderEditorTabs(ROOT_PANE_ID, 'missing', 0)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['a'])
  })
})

describe('pane-slice — setEditorTabPreview / setEditorTabPinned / clearEditorTabPreviewEverywhere', () => {
  it('setEditorTabPreview marks exactly one tab preview per pane', () => {
    const store = makeStoreWithBuffers([
      { id: 'a', type: 'editor', isPreview: false },
      { id: 'b', type: 'editor', isPreview: false },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'b', type: 'editor', name: 'b.ts' })

    actions.setEditorTabPreview(ROOT_PANE_ID, 'a')
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(true)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(false)

    actions.setEditorTabPreview(ROOT_PANE_ID, 'b')
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(false)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(true)
  })

  it('setEditorTabPinned sets isPinned on the tab content', () => {
    const store = makeStoreWithBuffers([{ id: 'a', type: 'editor', isPinned: false }])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })

    actions.setEditorTabPinned(ROOT_PANE_ID, 'a', true)
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPinned).toBe(true)

    actions.setEditorTabPinned(ROOT_PANE_ID, 'a', false)
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPinned).toBe(false)
  })

  it('clearEditorTabPreviewEverywhere clears preview on every tab, in every pane', () => {
    const store = makeStoreWithBuffers([
      { id: 'a', type: 'editor', isPreview: true },
      { id: 'b', type: 'editor', isPreview: true },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'b', type: 'editor', name: 'b.ts' })

    actions.clearEditorTabPreviewEverywhere()

    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(false)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(false)
  })
})

describe('pane-slice — switchToNextEditorTab / switchToPreviousEditorTab', () => {
  it('cycles forward through a pane’s tabs, wrapping at the end', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'b', type: 'editor', name: 'b.ts' })
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'a')

    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')

    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
  })

  it('cycles backward through a pane’s tabs, wrapping at the start', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'b', type: 'editor', name: 'b.ts' })
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'a')

    actions.switchToPreviousEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')
  })

  it('is a no-op with 0 or 1 tabs', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBeNull()

    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'a', type: 'editor', name: 'a.ts' })
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
  })
})

// C1 regression: removing/closing an editor tab must release its retained
// Monaco model via the editorManager (keyed by FILE URI), so closing a tab frees
// the model and a reopen reads fresh content. Wires a fake editorManager onto the
// store object (the same place `createWorkspaceStore` Object.assign's it).
describe('pane-slice → editorManager model release (C1)', () => {
  function makeStoreWithManager() {
    const closeBuffer = vi.fn()
    // The pane slice reads `get().buffers` (path lookup) and `api.editorManager`.
    type S = PaneSlice & {
      buffers: Array<{ id: string; type: string; path: string }>
    }
    let api: { editorManager: { closeBuffer: typeof closeBuffer } }
    const store = createStore<S>()(
      immer((set, get, rawApi) => {
        api = rawApi as unknown as typeof api
        api.editorManager = { closeBuffer }
        return {
          ...createPaneSlice(
            ...([set, get, rawApi] as unknown as Parameters<typeof createPaneSlice>),
          ),
          buffers: [{ id: 'tab-ed', type: 'editor', path: '/src/a.ts' }],
        }
      }),
    )
    return { store, closeBuffer }
  }

  it('removeEditorTabFromPane releases the model for that pane (paneId + fileUri)', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-ed',
      type: 'editor',
      name: 'a.ts',
    })
    store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-ed')
    expect(closeBuffer).toHaveBeenCalledWith(ROOT_PANE_ID, fileUri('/src/a.ts'))
  })

  it('does not release for a pane that never held the tab', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-ed')
    expect(closeBuffer).not.toHaveBeenCalled()
  })
})

// I4 regression (renamed): `activateEditorTabInPane` wrote whatever id it was
// handed straight onto the pane. Callers can hand it a DEAD one, and a pane
// pointed at a tab it doesn't hold renders its empty fallback while the tab
// strip still shows tabs, none of them selected.
describe('pane-slice — activateEditorTabInPane only activates something the pane holds (I4)', () => {
  it('ignores a tab id that this pane does not hold', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-gone')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-1')
  })

  it('ignores a tab that lives in a different pane', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-elsewhere',
      type: 'editor',
      name: 'b.ts',
    })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-elsewhere')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-1')
  })

  it('activates normally when the pane really holds the tab', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-2', type: 'editor', name: 'b.ts' })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-2')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-2')
    expect(actions.getPaneById(ROOT_PANE_ID)?.id).toBe(ROOT_PANE_ID)
  })
})

// I8 regression, restated: splitPane sharing a REAL tab id across panes still
// works (only the retired "New Tab" placeholder ever needed exemption from
// this, and that placeholder no longer exists).
describe('pane-slice — splitPane sharing a tab id across panes', () => {
  it('shares a real tab id across panes', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'tab-1', type: 'editor', name: 'a.ts' })

    const newPaneId = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')

    const newPane = actions.getPaneById(newPaneId!)
    expect(newPane?.editorTabIds).toEqual(['tab-1'])
  })
})

describe('pane-slice — closePane merges editor tabs, leaves chat untouched', () => {
  it('merges the closing split’s tabs into the surviving pane', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'root-tab', type: 'editor', name: 'a.ts' })
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addEditorTabToPane(splitId, { id: 'split-tab', type: 'editor', name: 'b.ts' })

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.editorTabIds).toEqual(['root-tab', 'split-tab'])
  })

  it('does not duplicate a tab id the survivor already holds', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, { id: 'shared-tab', type: 'editor', name: 'a.ts' })
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'shared-tab')!

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.editorTabIds).toEqual(['shared-tab'])
  })

  it('never merges the closing pane’s chat into the survivor', () => {
    const actions = makeStore().getState().paneActions
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.setPaneChat(splitId, 'chat-in-split', 'runner-1')

    actions.closePane(splitId)

    expect(actions.getPaneById(ROOT_PANE_ID)?.chatId).toBeNull()
  })

  // Spec §5.4's "empties rather than refuses" is for the true last pane only —
  // a split sibling that closes while another pane still holds the screen has
  // somewhere to go, so its row is deleted outright, not left behind empty.
  it('deletes a closing split sibling outright — the empty-stage treatment is only for the last pane', () => {
    const actions = makeStore().getState().paneActions
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.setPaneChat(splitId, 'chat-in-split', 'runner-1')

    actions.closePane(splitId)

    expect(actions.getPaneById(splitId)).toBeNull()
    expect(actions.getPaneById(ROOT_PANE_ID)).not.toBeNull()
  })
})

// Regression: closing a layout's sole leaf under its own canonical id (the
// common single-pane-workspace case) used to create a fresh empty PaneGroup
// at `fallbackId` and then immediately `delete` it again, because `paneId`
// and `fallbackId` are the same string here. `getPaneById` returned undefined
// afterward instead of the empty stage — see closePane's `else` branch. This
// is also the exact scenario spec §5.4 requires ("closing the last pane
// empties it rather than refusing") — the first test below IS that case.
describe('pane-slice — closing the sole root/bottom pane empties it, never deletes it', () => {
  it('closing the sole root pane leaves an empty PaneGroup at ROOT_PANE_ID', () => {
    const actions = makeStore().getState().paneActions
    actions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    actions.closePane(ROOT_PANE_ID)

    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root).not.toBeNull()
    expect(root?.chatId).toBeNull()
    expect(root?.editorTabIds).toEqual([])
  })

  it('closing the sole bottom pane leaves an empty PaneGroup at BOTTOM_PANE_ID', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, { id: 'tab-1', type: 'terminal', name: 'sh' })

    actions.closePane(BOTTOM_PANE_ID)

    const bottom = actions.getPaneById(BOTTOM_PANE_ID)
    expect(bottom).not.toBeNull()
    expect(bottom?.editorTabIds).toEqual([])
  })

  it('re-collapsing to a non-canonical sole leaf still empties the canonical id, not the stale one', () => {
    const actions = makeStore().getState().paneActions
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.closePane(ROOT_PANE_ID) // leaves splitId as the tree's sole leaf

    actions.closePane(splitId)

    expect(actions.getPaneById(ROOT_PANE_ID)).not.toBeNull()
    expect(actions.getPaneById(splitId)).toBeNull()
  })
})

// Spec §5.5: "the view dies, the row does not." dormantArrangements is what
// makes an idle close undoable; a chat the daemon is still working relies on
// agentChats.working alone and must not also get a dormant record.
//
// Isolated PaneSlice store carrying just enough of `agentChats.working` to
// exercise this — same pattern as `makeStoreWithBuffers` above, avoiding a
// full `createWorkspaceStore` (whose merged-slice `setState` type doesn't
// accept a void-returning immer callback here — a pre-existing tsc trap
// unrelated to this test).
function makeStoreWithWorking(working: Record<string, boolean>) {
  return createStore<PaneSlice & { agentChats: { working: Record<string, boolean> } }>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
      agentChats: { working },
    })),
  )
}

describe('pane-slice — dormantArrangements (spec §5.5)', () => {
  it('closing a pane holding an idle chat remembers it as dormant', () => {
    const store = makeStoreWithWorking({})
    const paneId = ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')

    store.getState().paneActions.closePane(paneId)

    expect(store.getState().dormantArrangements).toEqual([
      { id: paneId, chatIds: ['chat-1'], state: 'dormant' },
    ])
  })

  it('closing a pane holding a WORKING chat does not add a dormant record', () => {
    const store = makeStoreWithWorking({ 'chat-1': true })
    const paneId = ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')

    store.getState().paneActions.closePane(paneId)

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('closing a chatless pane adds no dormant record', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.closePane(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('is a no-op (never throws) on a bare pane-slice store with no agentChats', () => {
    const actions = makeStore().getState().paneActions
    actions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    expect(() => actions.closePane(ROOT_PANE_ID)).not.toThrow()
  })
})

// Spec §5.4: "on a remembered one → forgets the arrangement." The symmetric
// removal to the push `closePane` does above.
describe('pane-slice — forgetDormantArrangement (spec §5.4)', () => {
  it('removes the named arrangement, leaving the rest', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.closePane(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toHaveLength(1)

    store.getState().paneActions.forgetDormantArrangement(ROOT_PANE_ID)

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('is a no-op for an id that names no arrangement', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.closePane(ROOT_PANE_ID)

    store.getState().paneActions.forgetDormantArrangement('no-such-entry')

    expect(store.getState().dormantArrangements).toHaveLength(1)
  })
})

// Task 22: spec §8.2's merge/survivor bookkeeping — "merging opens... you get
// them side by side", "whatever goes up leaves every arrangement that was
// remembering it... the arrangement you leave is remembered minus whatever
// you took out of it", "an arrangement left with nobody in it goes."
describe('pane-slice — groupIntoArrangement (spec §8.2 "merging")', () => {
  it('groups two chats into one live entry', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual(['chat-1', 'chat-2'])
  })

  it('is a no-op for fewer than two chats', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1'])
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('extends an existing arrangement rather than nesting a second one — "grows instead of reopening"', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    const [{ id }] = store.getState().dormantArrangements

    store.getState().paneActions.groupIntoArrangement(['chat-2', 'chat-3'])

    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect(store.getState().dormantArrangements[0].id).toBe(id)
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual([
      'chat-1',
      'chat-2',
      'chat-3',
    ])
  })

  it('strips the incoming chats out of any OTHER arrangement they used to belong to', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-3'])
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual([
      'chat-1',
      'chat-2',
      'chat-3',
    ])
  })

  // Fix round 1 (real, reviewer-verified regression): the original
  // implementation always did filter-then-push, which re-inserted a GROWING
  // entry at the array TAIL — dropping an already-live set from wherever it
  // sat straight to the bottom of Recents on every merge. Spec §5.6: "an
  // arrangement that gains or loses a pane inherits the place of the ONE IT
  // GREW OUT OF."
  it('grows an entry IN PLACE — it does not jump to the end of the list (Fix round 1)', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['t', 'd']) // index 0
    store.getState().paneActions.groupIntoArrangement(['x', 'y']) // index 1

    store.getState().paneActions.groupIntoArrangement(['t', 'z']) // grows the FIRST entry

    const { dormantArrangements } = store.getState()
    expect(dormantArrangements).toHaveLength(2)
    // The grown set is still FIRST, not shoved to the end behind [x, y].
    expect([...dormantArrangements[0].chatIds].sort()).toEqual(['d', 't', 'z'])
    expect([...dormantArrangements[1].chatIds].sort()).toEqual(['x', 'y'])
  })

  // A brand-new group (no pre-existing entry to grow out of) has nowhere of
  // its own to inherit — appending is the only sensible slot for it.
  it('appends a genuinely brand-new group after whatever already exists', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['a', 'b'])

    store.getState().paneActions.groupIntoArrangement(['c', 'd'])

    const { dormantArrangements } = store.getState()
    expect(dormantArrangements).toHaveLength(2)
    expect([...dormantArrangements[0].chatIds].sort()).toEqual(['a', 'b'])
    expect([...dormantArrangements[1].chatIds].sort()).toEqual(['c', 'd'])
  })

  // Fix round 1: the "arrangement left with nobody in it goes" rule (§8.2)
  // IS still reachable — not through setPaneChat pulling a group down to its
  // LAST member (see the describe block below, where a shrunk-to-one entry
  // is deliberately protected instead — it is now that chat's own slot) —
  // but here: that already-single-member slot gets swept up whole into a
  // DIFFERENT merge that doesn't choose it as the owner, and the now-empty
  // record it leaves behind is pruned.
  it('an arrangement left with nobody in it goes, when its sole member joins a different merge', () => {
    const store = makeStore()
    // An EARLIER entry, so the owner search below finds IT first.
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    // chat-6's own slot: a pair that gets pulled back down to one member.
    store.getState().paneActions.groupIntoArrangement(['chat-6', 'chat-7'])
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-7', null)
    expect([...store.getState().dormantArrangements].map((e) => [...e.chatIds].sort())).toEqual([
      ['chat-1', 'chat-2'],
      ['chat-6'],
    ])

    // chat-6 now joins a merge with chat-1 — the [chat-1, chat-2] entry (it
    // comes first, and it already owns chat-1) is the one that grows;
    // chat-6's own single-member entry has nobody left once chat-6 leaves it.
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-6'])

    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual([
      'chat-1',
      'chat-2',
      'chat-6',
    ])
  })
})

describe('pane-slice — setPaneChat sheds stale arrangement membership (spec §8.2)', () => {
  it('a chat moving fresh into a NEW pane leaves every arrangement that remembered it, survivors kept as a set', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2', 'chat-3'])
    const otherPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!

    // Some other mechanism re-homes chat-2 onto a pane that isn't already
    // showing it — the exact bookkeeping spec §8.2 describes ("pull one chat
    // out of a live three-up, the other two are kept as a set, not the
    // three"), exercised directly against the one write path for what a pane
    // holds rather than through a gesture, since no live UI path moves an
    // already-open chat today (`performSidebarPaneDrop` reveals it in place
    // instead — see its own test file's note on this).
    store.getState().paneActions.setPaneChat(otherPane, 'chat-2', null)

    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual(['chat-1', 'chat-3'])
  })

  // Fix round 1 (real, reviewer-verified regression): the original version
  // of this stripped a SINGLE-chat entry exactly like a multi-chat one —
  // restoring a dormant chat (`openAgentChat`, reachable from New Tab's
  // recent list) deleted its own dormant record outright, so
  // `deriveRecentsEntries` re-derived the row fresh from the pane loop,
  // appended AFTER every remaining dormant entry instead of staying at its
  // slot. Spec §5.6: "Restoring a dormant one — the row stays exactly where
  // it sits." A single-chat entry IS that chat's own persistent slot now, so
  // `setPaneChat` never strips it down past one member — it stays, and just
  // recomputes to 'live' in place the next time Recents derives.
  it('pulling the last member out of a pair leaves the SURVIVOR its own single-chat slot — not removed', () => {
    const store = makeStore()
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    const paneA = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!

    // chat-1 moves out first — the pair's own entry sheds it (2 members, so
    // still eligible) and is left with chat-2 alone.
    store.getState().paneActions.setPaneChat(paneA, 'chat-1', null)
    expect(store.getState().dormantArrangements).toEqual([
      { id: expect.any(String), chatIds: ['chat-2'], state: 'live' },
    ])

    // Now chat-2 ALSO moves — but its entry is down to ONE member, so
    // setPaneChat leaves it alone entirely (Fix round 1): it is chat-2's own
    // slot now, not a "set" with nobody left in it, and the SAME record
    // just recomputes to 'live' at chat-2's new pane on the next derive.
    const paneB = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    const before = store.getState().dormantArrangements
    store.getState().paneActions.setPaneChat(paneB, 'chat-2', null)

    expect(store.getState().dormantArrangements).toBe(before)
    expect(store.getState().dormantArrangements).toEqual([
      { id: expect.any(String), chatIds: ['chat-2'], state: 'live' },
    ])
  })

  it('restoring a dormant chat via setPaneChat keeps its record — and its slot — instead of deleting it (spec §5.6)', () => {
    const store = makeStoreWithWorking({})
    // A single-chat entry, exactly as closePane's own dormant push creates.
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.closePane(ROOT_PANE_ID)
    const [dormantRecord] = store.getState().dormantArrangements
    expect(dormantRecord).toEqual({ id: ROOT_PANE_ID, chatIds: ['chat-1'], state: 'dormant' })

    const before = store.getState().dormantArrangements
    // Restore it into a pane — the same write `openAgentChat` performs.
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)

    // The SAME record, same id, same slot — not deleted and re-derived
    // fresh (which would have appended it after every other dormant entry).
    // Referentially the SAME array too: nothing here needed stripping, so
    // nothing should have minted a new one for React to re-render over.
    expect(store.getState().dormantArrangements).toBe(before)
    expect(store.getState().dormantArrangements).toEqual([dormantRecord])
  })

  it('re-setting the SAME chat a pane already holds does not touch dormantArrangements', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.groupIntoArrangement(['chat-2', 'chat-3']) // unrelated set
    const before = store.getState().dormantArrangements

    // Same chatId ROOT already holds, just a new runner (e.g. /resume) — no
    // MOVE happened, so nothing should be stripped from anywhere.
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-2')

    expect(store.getState().dormantArrangements).toBe(before)
  })

  it('clearing a pane (chatId → null) does not strip that chat from an arrangement', () => {
    // Only a chat moving somewhere NEW sheds membership — a bare clear is not
    // a move (it is not "going up" anywhere), so it leaves the arrangement
    // exactly as closePane's own dormant push already assumes it will.
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)

    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual(['chat-1', 'chat-2'])
  })
})

describe('pane-slice — closePane defers to an existing arrangement instead of duplicating', () => {
  it('does not push a second, single-chat record for a chat already remembered by a merged set', () => {
    const store = makeStoreWithWorking({})
    // Mirrors `performSidebarPaneDrop`'s own merge order — `setPaneChat`
    // FIRST (chat-1 enters ROOT fresh, nothing to strip yet), THEN
    // `groupIntoArrangement` (now that it is actually resident there).
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])

    store.getState().paneActions.closePane(ROOT_PANE_ID)

    // Still exactly the one entry — closing chat-1's pane did not mint a
    // duplicate {id: ROOT_PANE_ID, chatIds: ['chat-1']} alongside it.
    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual(['chat-1', 'chat-2'])
  })
})
