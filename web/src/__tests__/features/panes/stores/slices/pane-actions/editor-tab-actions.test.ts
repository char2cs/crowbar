import { describe, it, expect, afterEach, vi } from 'vitest'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { showingLayout } from '@/features/panes/lib/view-state'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

type Buf = { id: string; type: string; workspaceId: string; [k: string]: unknown }
const withBuffers = (buffers: Buf[]) => makePaneOnlyStore({ buffers })

describe('editor tabs in a pane', () => {
  it('addEditorTabToPane adds, activates and opens the editor view', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    const pane = actions.getPaneById(ROOT_PANE_ID)
    expect(pane?.editorTabIds).toContain('tab-1')
    expect(pane?.activeEditorTabId).toBe('tab-1')
    expect(pane?.editorOpen).toBe(true)
  })

  it('does not reopen a split the user toggled off once the pane holds a tab', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    store.setState((s) => {
      s.panes[ROOT_PANE_ID].editorOpen = false
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-2'))
    const pane = actions.getPaneById(ROOT_PANE_ID)
    expect(pane?.editorTabIds).toEqual(['tab-1', 'tab-2'])
    expect(pane?.activeEditorTabId).toBe('tab-2')
    expect(pane?.editorOpen).toBe(false)
  })

  it('activateChatInPane selects the chat without clearing activeEditorTabId', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.openChat('chat-1')
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    expect(actions.getPaneById(ROOT_PANE_ID)?.chatSelected).toBe(false)

    actions.activateChatInPane(ROOT_PANE_ID)

    const pane = actions.getPaneById(ROOT_PANE_ID)
    expect(pane?.chatSelected).toBe(true)
    expect(pane?.activeEditorTabId).toBe('tab-1')
    expect(pane?.editorOpen).toBe(true)
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activateEditorTabInPane clears chatSelected', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.openChat('chat-1')
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    actions.activateChatInPane(ROOT_PANE_ID)
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-1')
    expect(actions.getPaneById(ROOT_PANE_ID)?.chatSelected).toBe(false)
  })

  it('removeEditorTabFromPane removes the tab and closes the view with the last one', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-2'))
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['tab-2'])
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-2')
    const pane = actions.getPaneById(ROOT_PANE_ID)
    expect(pane?.editorTabIds).toEqual([])
    expect(pane?.activeEditorTabId).toBeNull()
    expect(pane?.editorOpen).toBe(false)
  })

  it('closing the active tab activates the ADJACENT tab (right, else left)', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    for (const id of ['tab-1', 'tab-2', 'tab-3'])
      actions.addEditorTabToPane(ROOT_PANE_ID, editorTab(id))
    const activeOf = () => actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-2')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-2')
    expect(activeOf()).toBe('tab-3')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-3')
    expect(activeOf()).toBe('tab-1')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    expect(activeOf()).toBeNull()
  })

  it('never activates a tab whose content no longer exists', () => {
    const store = withBuffers([
      { id: 'tab-real', type: 'terminal', workspaceId: 'ws-test' },
      { id: 'tab-active', type: 'terminal', workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    for (const id of ['tab-real', 'tab-ghost', 'tab-active']) {
      actions.addEditorTabToPane(ROOT_PANE_ID, editorTab(id, 'terminal'))
    }
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-active')
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-active')
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-real')
  })

  it('losing the last tab collapses a chatless split pane out of the layout', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    const split = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')!
    actions.removeEditorTabFromPane(split, 'tab-1')
    expect(actions.getPaneById(split)).toBeNull()
    expect(getAllLeafIds(showingLayout(store.getState()))).toEqual([ROOT_PANE_ID])
  })

  it('a pane holding a chat keeps its place when its last tab closes', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.openChat('chat-1')
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    expect(actions.getPaneById(ROOT_PANE_ID)?.chatId).toBe('chat-1')
  })
})

describe('bottom tray tabs', () => {
  it('add / activate / remove work on the bottom pane', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('tab-1', 'terminal'))
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('tab-2', 'terminal'))
    actions.activateEditorTabInPane(BOTTOM_PANE_ID, 'tab-1')
    expect(actions.getPaneById(BOTTOM_PANE_ID)?.activeEditorTabId).toBe('tab-1')
    actions.removeEditorTabFromPane(BOTTOM_PANE_ID, 'tab-1')
    expect(actions.getPaneById(BOTTOM_PANE_ID)?.editorTabIds).toEqual(['tab-2'])
    expect(actions.getPaneByEditorTabId('tab-2')?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveEditorTabToPane moves a tab across trees (bottom -> stage)', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('tab-x', 'terminal'))
    actions.moveEditorTabToPane('tab-x', BOTTOM_PANE_ID, ROOT_PANE_ID)
    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.editorTabIds).toContain('tab-x')
    expect(root?.activeEditorTabId).toBe('tab-x')
    expect(root?.editorOpen).toBe(true)
    const bottom = actions.getPaneById(BOTTOM_PANE_ID)
    // Left empty, not force-closed — that belongs to closePane.
    expect(bottom?.editorTabIds).toEqual([])
    expect(bottom?.editorOpen).toBe(false)
  })
})

describe('reorder / preview / pinned / cycling', () => {
  it('reorderEditorTabs moves a tab to the target index; unknown tab is a no-op', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    for (const id of ['a', 'b', 'c']) actions.addEditorTabToPane(ROOT_PANE_ID, editorTab(id))
    actions.reorderEditorTabs(ROOT_PANE_ID, 'a', 2)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['b', 'c', 'a'])
    actions.reorderEditorTabs(ROOT_PANE_ID, 'missing', 0)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['b', 'c', 'a'])
  })

  it('setEditorTabPreview marks exactly one preview per pane', () => {
    const store = withBuffers([
      { id: 'a', type: 'editor', isPreview: false, workspaceId: 'ws-test' },
      { id: 'b', type: 'editor', isPreview: false, workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('a'))
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('b'))
    const preview = (id: string) => store.getState().buffers.find((b) => b.id === id)?.isPreview
    actions.setEditorTabPreview(ROOT_PANE_ID, 'a')
    expect([preview('a'), preview('b')]).toEqual([true, false])
    actions.setEditorTabPreview(ROOT_PANE_ID, 'b')
    expect([preview('a'), preview('b')]).toEqual([false, true])
  })

  it('setEditorTabPinned sets isPinned; clearEditorTabPreviewEverywhere clears every pane', () => {
    const store = withBuffers([
      { id: 'a', type: 'editor', isPinned: false, isPreview: true, workspaceId: 'ws-test' },
      { id: 'b', type: 'editor', isPreview: true, workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('a'))
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('b'))
    actions.setEditorTabPinned(ROOT_PANE_ID, 'a', true)
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPinned).toBe(true)
    actions.clearEditorTabPreviewEverywhere()
    expect(store.getState().buffers.every((b) => b.isPreview === false)).toBe(true)
  })

  it('switchToNext/PreviousEditorTab cycle and wrap; a no-op with fewer than two tabs', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBeNull()
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('a'))
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('b'))
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'a')
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
    actions.switchToPreviousEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')
  })
})

describe('activateEditorTabInPane only activates a tab the pane holds (I4)', () => {
  it('ignores an id the pane does not hold, including one in another pane', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('tab-elsewhere'))
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-gone')
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-elsewhere')
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-1')
  })
})

describe('editorManager model release (C1)', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  async function storeWithManager() {
    const closeBuffer = vi.fn()
    const registry = await import('@/features/workspace/stores/workspace-store-registry')
    vi.spyOn(registry, 'getWorkspaceStore').mockReturnValue({
      editorManager: { closeBuffer },
    } as unknown as ReturnType<typeof registry.getWorkspaceStore>)
    const store = withBuffers([
      { id: 'tab-ed', type: 'editor', path: '/src/a.ts', workspaceId: 'ws-test' },
    ])
    return { store, closeBuffer }
  }

  it('removeEditorTabFromPane releases the model for that pane (paneId + fileUri)', async () => {
    const { store, closeBuffer } = await storeWithManager()
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-ed'))
    store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-ed')
    expect(closeBuffer).toHaveBeenCalledWith(ROOT_PANE_ID, fileUri('ws-test', '/src/a.ts'))
  })

  it('does not release for a pane that never held the tab', async () => {
    const { store, closeBuffer } = await storeWithManager()
    store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-ed')
    expect(closeBuffer).not.toHaveBeenCalled()
  })
})
