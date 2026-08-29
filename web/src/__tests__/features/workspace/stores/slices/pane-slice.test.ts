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
})

// Regression: closing a layout's sole leaf under its own canonical id (the
// common single-pane-workspace case) used to create a fresh empty PaneGroup
// at `fallbackId` and then immediately `delete` it again, because `paneId`
// and `fallbackId` are the same string here. `getPaneById` returned undefined
// afterward instead of the empty stage — see closePane's `else` branch.
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
