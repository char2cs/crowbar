// web/src/__tests__/features/panes/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/panes/stores/slices/pane-slice'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  getOrCreateWorkspaceStore,
  getAllActiveWorkspaceIds,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { deriveRecentsEntries } from '@/components/sidebar/lib/recents-entries'
import { viewIdOf } from '@/features/panes/lib/pane-views'
import { openAgentChat } from '@/features/agent/lib/open-agent-chat'

// Task 26: `agentChats.working` lives on the per-workspace store now (it never
// moved), while panes read it through `isChatWorking` (workspace-store-
// registry.ts), which searches every REGISTERED workspace store. Seeding a
// real per-workspace store via the real registry — rather than mocking
// `isChatWorking` — exercises the real integration and needs no mock.
afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetWindowPaneStoreForTests()
})

function makeStore() {
  return createStore<PaneSlice>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
    })),
  )
}

/**
 * Put a multi-chat Recents entry into `dormantArrangements` directly.
 *
 * There is no longer an ACTION that writes one: grouping used to be recorded
 * twice — here, by `groupIntoArrangement`, and (not at all) in the pane
 * layout — and `viewId` on the panes is the single grouping fact now (see
 * `pane-views.ts`). `RecentsEntry.chatIds` is still a list, so the survivor-
 * stripping rules below still have a shape to defend; seeding it is the
 * honest way to say "given such an entry exists" rather than routing through
 * an action that no longer means that.
 */
function seedArrangement(store: ReturnType<typeof makeStore>, id: string, chatIds: string[]): void {
  store.setState((s) => {
    s.dormantArrangements.push({ id, chatIds, state: 'live' })
  })
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
      workspaceId: 'ws-test',
    })
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).toContain('tab-1')
    expect(rootGroup?.activeEditorTabId).toBe('tab-1')
    expect(rootGroup?.editorOpen).toBe(true)
  })

  it('removeEditorTabFromPane removes the tab from the group', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-2',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).not.toContain('tab-1')
    expect(rootGroup?.editorTabIds).toContain('tab-2')
  })

  it('removeEditorTabFromPane closes the editor view once the last tab is gone', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-1')
    const rootGroup = actions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.editorTabIds).toEqual([])
    expect(rootGroup?.activeEditorTabId).toBeNull()
    expect(rootGroup?.editorOpen).toBe(false)
  })

  it('closing the active tab activates the ADJACENT tab (right neighbor, else left when last)', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-2',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-3',
      type: 'editor',
      name: 'c.ts',
      workspaceId: 'ws-test',
    })
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
      { id: 'tab-real', type: 'terminal', workspaceId: 'ws-test' },
      { id: 'tab-active', type: 'terminal', workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-real',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-ghost',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-active',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })

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
    store.getState().paneActions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-1',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
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
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-bottom',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
    actions.activateEditorTabInPane(BOTTOM_PANE_ID, 'tab-bottom')
    const bottomGroup = actions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.activeEditorTabId).toBe('tab-bottom')
  })

  it('removeEditorTabFromPane removes tab from bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-1',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-2',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
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
      workspaceId: 'ws-test',
    })
    const pane = store.getState().paneActions.getPaneByEditorTabId('tab-bottom')
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveEditorTabToPane moves a tab across trees (bottomRoot -> paneRoot)', () => {
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-x',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })
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
    const store = makeStore()
    const paneId = store.getState().panes[ROOT_PANE_ID]?.id ?? ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')
    expect(store.getState().paneActions.getPaneById(paneId)?.chatId).toBe('chat-1')
    store.getState().paneActions.setPaneChat(paneId, 'chat-2', 'runner-2')
    expect(store.getState().paneActions.getPaneById(paneId)?.chatId).toBe('chat-2')
    expect(store.getState().paneActions.getPaneById(paneId)?.runnerId).toBe('runner-2')
  })

  it('editor tabs are independent of the chat', () => {
    const store = makeStore()
    const paneId = store.getState().panes[ROOT_PANE_ID]?.id ?? ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')
    store.getState().paneActions.addEditorTabToPane(paneId, {
      id: 'file-1',
      type: 'editor',
      name: 'foo.ts',
      workspaceId: 'ws-test',
    })
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
      workspaceId: 'ws-test',
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
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'c',
      type: 'editor',
      name: 'c.ts',
      workspaceId: 'ws-test',
    })

    actions.reorderEditorTabs(ROOT_PANE_ID, 'a', 2)

    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['b', 'c', 'a'])
  })

  it('is a no-op when the tab is not in the pane', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.reorderEditorTabs(ROOT_PANE_ID, 'missing', 0)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['a'])
  })
})

describe('pane-slice — setEditorTabPreview / setEditorTabPinned / clearEditorTabPreviewEverywhere', () => {
  it('setEditorTabPreview marks exactly one tab preview per pane', () => {
    const store = makeStoreWithBuffers([
      { id: 'a', type: 'editor', isPreview: false, workspaceId: 'ws-test' },
      { id: 'b', type: 'editor', isPreview: false, workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })

    actions.setEditorTabPreview(ROOT_PANE_ID, 'a')
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(true)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(false)

    actions.setEditorTabPreview(ROOT_PANE_ID, 'b')
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(false)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(true)
  })

  it('setEditorTabPinned sets isPinned on the tab content', () => {
    const store = makeStoreWithBuffers([
      { id: 'a', type: 'editor', isPinned: false, workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })

    actions.setEditorTabPinned(ROOT_PANE_ID, 'a', true)
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPinned).toBe(true)

    actions.setEditorTabPinned(ROOT_PANE_ID, 'a', false)
    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPinned).toBe(false)
  })

  it('clearEditorTabPreviewEverywhere clears preview on every tab, in every pane', () => {
    const store = makeStoreWithBuffers([
      { id: 'a', type: 'editor', isPreview: true, workspaceId: 'ws-test' },
      { id: 'b', type: 'editor', isPreview: true, workspaceId: 'ws-test' },
    ])
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })

    actions.clearEditorTabPreviewEverywhere()

    expect(store.getState().buffers.find((b) => b.id === 'a')?.isPreview).toBe(false)
    expect(store.getState().buffers.find((b) => b.id === 'b')?.isPreview).toBe(false)
  })
})

describe('pane-slice — switchToNextEditorTab / switchToPreviousEditorTab', () => {
  it('cycles forward through a pane’s tabs, wrapping at the end', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'a')

    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')

    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
  })

  it('cycles backward through a pane’s tabs, wrapping at the start', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })
    actions.activateEditorTabInPane(ROOT_PANE_ID, 'a')

    actions.switchToPreviousEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('b')
  })

  it('is a no-op with 0 or 1 tabs', () => {
    const store = makeStore()
    const actions = store.getState().paneActions
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBeNull()

    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.switchToNextEditorTab(ROOT_PANE_ID)
    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('a')
  })
})

// C1 regression: removing/closing an editor tab must release its retained
// Monaco model via the editorManager (keyed by FILE URI), so closing a tab frees
// the model and a reopen reads fresh content. Wires a fake editorManager onto the
// store object (the same place `createWorkspaceStore` Object.assign's it).
describe('pane-slice → editorManager model release (C1)', () => {
  // Task 26: the editor manager is resolved BY WORKSPACE now
  // (`getWorkspaceStore(buf.workspaceId)?.editorManager`, since Monaco models
  // stay per-workspace to avoid mixing two retained workspaces'
  // same-relative-path files) rather than read straight off the slice's own
  // `api` — mock the registry lookup instead of stubbing `api`. Fix round 1
  // (I3): this must be the non-creating `getWorkspaceStore`, never
  // `getOrCreateWorkspaceStore` — the latter would silently re-register a
  // store for an already-evicted workspace and leak it for the session.
  afterEach(() => {
    vi.restoreAllMocks()
  })

  async function makeStoreWithManager() {
    const closeBuffer = vi.fn()
    const registry = await import('@/features/workspace/stores/workspace-store-registry')
    vi.spyOn(registry, 'getWorkspaceStore').mockReturnValue({
      editorManager: { closeBuffer },
    } as unknown as ReturnType<typeof registry.getWorkspaceStore>)

    type S = PaneSlice & {
      buffers: Array<{ id: string; type: string; path: string; workspaceId: string }>
    }
    const store = createStore<S>()(
      immer((set, get, rawApi) => ({
        ...createPaneSlice(
          ...([set, get, rawApi] as unknown as Parameters<typeof createPaneSlice>),
        ),
        buffers: [{ id: 'tab-ed', type: 'editor', path: '/src/a.ts', workspaceId: 'ws-test' }],
      })),
    )
    return { store, closeBuffer }
  }

  it('removeEditorTabFromPane releases the model for that pane (paneId + fileUri)', async () => {
    const { store, closeBuffer } = await makeStoreWithManager()
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-ed',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-ed')
    expect(closeBuffer).toHaveBeenCalledWith(ROOT_PANE_ID, fileUri('/src/a.ts'))
  })

  it('does not release for a pane that never held the tab', async () => {
    const { store, closeBuffer } = await makeStoreWithManager()
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
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-gone')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-1')
  })

  it('ignores a tab that lives in a different pane', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-elsewhere',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })

    actions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-elsewhere')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('tab-1')
  })

  it('activates normally when the pane really holds the tab', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-2',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })

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
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'tab-1',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })

    const newPaneId = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')

    const newPane = actions.getPaneById(newPaneId!)
    expect(newPane?.editorTabIds).toEqual(['tab-1'])
  })
})

describe('pane-slice — closePane merges editor tabs, leaves chat untouched', () => {
  it('merges the closing split’s tabs into the surviving pane', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'root-tab',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addEditorTabToPane(splitId, {
      id: 'split-tab',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'ws-test',
    })

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.editorTabIds).toEqual(['root-tab', 'split-tab'])
  })

  it('does not duplicate a tab id the survivor already holds', () => {
    const actions = makeStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'shared-tab',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'ws-test',
    })
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
    actions.addEditorTabToPane(BOTTOM_PANE_ID, {
      id: 'tab-1',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })

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

/**
 * `addPane` — spec §8.4's "clicking a chat makes its own view", the primitive
 * a CLICK opens through. `splitPane` answers §8.1's drag-drop question
 * instead, and carves the new pane out of the one it is handed; a click that
 * borrowed it made every new view a subdivision of whichever pane happened to
 * be active.
 */
describe('pane-slice — addPane (spec §8.4)', () => {
  it('adds an empty, active pane without touching what the other panes hold', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const added = store.getState().paneActions.addPane()!

    expect(added).not.toBe(ROOT_PANE_ID)
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().panes[added].chatId).toBeNull()
    expect(store.getState().activePaneId).toBe(added)
  })

  it('takes the whole screen — the views before it are parked, never tiled beside it', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const second = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(second, 'chat-2', null)
    const third = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(third, 'chat-3', null)

    // THE BUG THIS FEATURE EXISTS FOR. `addPane` used to append a peer leaf to
    // the one shared tree, so three separately clicked chats drew as three
    // columns at once — genuinely independent views in the data, still tiled
    // on screen. Only the newest occupies the content area now.
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([third])
    expect(store.getState().activeViewId).toBe(third)

    // Parked, not closed: the other two are whole, off screen, and still hold
    // their chats.
    expect(Object.keys(store.getState().parkedViews).sort()).toEqual([ROOT_PANE_ID, second].sort())
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().panes[second].chatId).toBe('chat-2')
  })

  it('a view switched back to is the same arrangement it was parked as', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    // A real MERGE, so the parked view has more than one pane to preserve.
    const mate = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(mate, 'chat-2', null)
    const merged = store.getState().rootLayout

    const solo = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(solo, 'chat-3', null)
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([solo])

    store.getState().paneActions.activateView(ROOT_PANE_ID)

    expect(store.getState().rootLayout).toEqual(merged)
    expect(store.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(getAllLeafIds(store.getState().rootLayout).sort()).toEqual([ROOT_PANE_ID, mate].sort())
    // And the view it left is now the parked one.
    expect(Object.keys(store.getState().parkedViews)).toEqual([solo])
  })

  it('the empty stage is never parked — it is a fallback, not a view to switch back to', () => {
    const store = makeStore()
    // Nothing open: rootLayout is the bare empty root pane.
    const opened = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(opened, 'chat-1', null)

    expect(store.getState().parkedViews).toEqual({})
    expect(store.getState().panes[ROOT_PANE_ID]).toBeUndefined()
  })

  it('is a ROOT-layout primitive — the bottom panel is never where a click lands', () => {
    const store = makeStore()

    const added = store.getState().paneActions.addPane()!

    expect(getAllLeafIds(store.getState().rootLayout)).toContain(added)
    expect(getAllLeafIds(store.getState().bottomLayout)).toEqual([BOTTOM_PANE_ID])
  })
})

/**
 * "The empty case ... should only appear when NO VIEW is opened. It's just a
 * fallback when nothing is found, not a normal view." A pane that loses the
 * last thing it held leaves the layout, collapsing into its sibling exactly as
 * closing it would — the one exception being the last pane in its own tree,
 * which IS that fallback screen (spec §5.4: "closing the last pane empties it
 * rather than refusing").
 */
describe('pane-slice — an emptied pane leaves the layout', () => {
  it('a pane cleared to no chat collapses out of a split', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const second = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(second, 'chat-2', 'runner-2')

    // The eviction `use-workspace-agent-chats-stream.ts`'s followRunner runs
    // when a runner walks onto a chat another pane was showing.
    store.getState().paneActions.setPaneChat(second, null, null)

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(store.getState().panes[second]).toBeUndefined()
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
  })

  it('hands focus to a surviving pane when the collapsed one was active', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const second = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(second, 'chat-2', 'runner-2')
    expect(store.getState().activePaneId).toBe(second)

    store.getState().paneActions.setPaneChat(second, null, null)

    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(store.getState().mostRecentActivePaneIds).not.toContain(second)
  })

  it('the LAST pane stays, empty — that one is the fallback screen, not a view', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
  })

  it('a pane moving ONTO a chat never disturbs the layout', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const second = store.getState().paneActions.addPane()!

    // The gap `addPane` leaves — an empty pane its caller is about to fill —
    // must survive long enough to be filled. It is the SHOWING view's only
    // pane now, so a sweep that treated "empty" as "collapse me" would take
    // the whole new view down before its chat ever arrived.
    store.getState().paneActions.setPaneChat(second, 'chat-2', null)

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([second])
    expect(store.getState().panes[second].chatId).toBe('chat-2')
    // The view it opened in front of is untouched, off screen.
    expect(Object.keys(store.getState().parkedViews)).toEqual([ROOT_PANE_ID])
  })

  it('losing the last editor tab collapses a chatless pane out of the layout', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const second = store.getState().paneActions.addPane()!
    store.getState().paneActions.addEditorTabToPane(second, {
      id: 'tab-1',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })

    store.getState().paneActions.removeEditorTabFromPane(second, 'tab-1')

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
  })

  it('a pane still holding a chat keeps its place when its last editor tab closes', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const second = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(second, 'chat-2', null)
    store.getState().paneActions.addEditorTabToPane(second, {
      id: 'tab-1',
      type: 'terminal',
      name: 'sh',
      workspaceId: 'ws-test',
    })

    store.getState().paneActions.removeEditorTabFromPane(second, 'tab-1')

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([second])
    expect(store.getState().panes[second].chatId).toBe('chat-2')
  })
})

// Spec §5.5 USED to say "the view dies, the row does not" — a plain close
// left a dormant, click-to-reopen record. Removed: "the pane disappears, but
// its row on recents don't — I have to hit the X once again to then have it
// removed from recents" (verbatim). A close is now final on both sides; the
// Recents row goes with the pane, in the SAME action.
//
// Task 26: `isChatWorking` (pane-slice.ts's own read of "is this chat
// mid-turn") searches every REGISTERED workspace store's real
// `agentChats.working` — seed one for real via the registry rather than
// faking a local `agentChats` field the slice no longer reads directly.
function makeStoreWithWorking(working: Record<string, boolean>) {
  const wsStore = getOrCreateWorkspaceStore('ws-working-test')
  for (const [chatId, isWorking] of Object.entries(working)) {
    wsStore.getState().setAgentChatWorking(chatId, isWorking)
  }
  return makeStore()
}

describe('pane-slice — closing a view leaves no dormant record (spec §5.5)', () => {
  it('closing a pane holding an idle chat leaves it out of Recents entirely — no second close needed', () => {
    const store = makeStoreWithWorking({})
    const paneId = ROOT_PANE_ID
    store.getState().paneActions.setPaneChat(paneId, 'chat-1', 'runner-1')

    store.getState().paneActions.closePane(paneId)

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('closing a pane holding a WORKING chat does not add a dormant record either', () => {
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

  // setPaneChat's own hotswap-away archive (spec §8.4) is a SEPARATE
  // mechanism, untouched by this — a chat can still predate this pane's
  // close with an existing dormant record. Closing purges it too, rather
  // than leaving a stale row `deriveRecentsEntries` would silently revive.
  it('closing a chat that already had a stale dormant record (from an earlier hotswap-away) purges that too', () => {
    const store = makeStoreWithWorking({})
    const { paneActions } = store.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2') // archives chat-1 as dormant
    expect(store.getState().dormantArrangements).toHaveLength(1)
    // chat-1 comes back — into its OWN, still-empty pane, not swapped back
    // over chat-2 (which would just re-archive chat-2 instead, per §8.4).
    const other = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    paneActions.setPaneChat(other, 'chat-1', 'runner-1')

    paneActions.closePane(other) // closes chat-1 — its OWN stale record must go too

    expect(store.getState().dormantArrangements).toEqual([])
  })
})

// Spec §5.4: "on a remembered one → forgets the arrangement." Still reachable
// off a record some OTHER path left behind (setPaneChat's own hotswap-away
// archive, spec §8.4) — closePane no longer creates one of its own to forget.
describe('pane-slice — forgetDormantArrangement (spec §5.4)', () => {
  function archiveChatOne(store: ReturnType<typeof makeStoreWithWorking>) {
    const { paneActions } = store.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2') // archives chat-1
    return store.getState().dormantArrangements[0]!.id
  }

  it('removes the named arrangement, leaving the rest', () => {
    const store = makeStoreWithWorking({})
    const entryId = archiveChatOne(store)
    expect(store.getState().dormantArrangements).toHaveLength(1)

    store.getState().paneActions.forgetDormantArrangement(entryId)

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('is a no-op for an id that names no arrangement', () => {
    const store = makeStoreWithWorking({})
    archiveChatOne(store)

    store.getState().paneActions.forgetDormantArrangement('no-such-entry')

    expect(store.getState().dormantArrangements).toHaveLength(1)
  })
})

// Spec §9: deletion is the only act that removes a THING rather than a view,
// "so it is the only one that can leave a name behind. It clears the layout of
// any pane holding a deleted chat, plucks every arrangement in Recents that
// remembered one, drops arrangements left empty."
describe('pane-slice — forgetChat (spec §9)', () => {
  it('clears every pane holding the deleted chat and leaves the others alone', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.setPaneChat(BOTTOM_PANE_ID, 'chat-2', 'runner-2')

    store.getState().paneActions.forgetChat('chat-1')

    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
    expect(store.getState().panes[ROOT_PANE_ID].runnerId).toBeNull()
    expect(store.getState().panes[BOTTOM_PANE_ID].chatId).toBe('chat-2')
  })

  it('plucks the chat out of a remembered SET, keeping the survivors grouped', () => {
    const store = makeStoreWithWorking({})
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2', 'chat-3'])
    const entryId = store.getState().dormantArrangements[0].id

    store.getState().paneActions.forgetChat('chat-2')

    const [entry] = store.getState().dormantArrangements
    expect(entry.id).toBe(entryId) // the survivors keep their slot (§5.6)
    expect(entry.chatIds).toEqual(['chat-1', 'chat-3'])
  })

  it('drops an arrangement left with nobody in it', () => {
    const store = makeStoreWithWorking({})
    const { paneActions } = store.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2') // archives chat-1 as dormant
    expect(store.getState().dormantArrangements).toHaveLength(1)

    store.getState().paneActions.forgetChat('chat-1')

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('never archives the deleted chat — a ghost row is exactly what §9 forbids', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.forgetChat('chat-1')

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('is a no-op for a chat nothing holds or remembers', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const before = store.getState().dormantArrangements

    store.getState().paneActions.forgetChat('no-such-chat')

    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().dormantArrangements).toBe(before)
  })

  it('an emptied last pane falls back to the first chat still standing (spec §9)', () => {
    const store = makeStoreWithWorking({})
    const { paneActions } = store.getState()
    // chat-2 gets its own remembered (dormant) slot the moment chat-1
    // replaces it in the same pane (setPaneChat's own hotswap-away archive,
    // spec §8.4) — a chat that survives this deletion, "still standing".
    // chat-1 ends up the only LIVE view, in the window's sole pane.
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    expect(store.getState().dormantArrangements).toEqual([
      { id: expect.any(String), chatIds: ['chat-2'], state: 'dormant' },
    ])

    store.getState().paneActions.forgetChat('chat-1')

    // The last pane does not go blank — it picks up the first chat still
    // standing instead of the empty stage.
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-2')
  })

  it('does not reach for a fallback when another pane still holds something', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.setPaneChat(BOTTOM_PANE_ID, 'chat-2', 'runner-2')

    store.getState().paneActions.forgetChat('chat-1')

    // Ordinary close-to-empty-stage — chat-2's own pane means this was not
    // "the last pane".
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
    expect(store.getState().panes[BOTTOM_PANE_ID].chatId).toBe('chat-2')
  })

  it('leaves the last pane empty when nothing else is remembered either', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.forgetChat('chat-1')

    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
  })
})

// Task 22 / the View model: spec §8.2's merge and survivor rules. The merge
// itself is no longer a Recents write at all — `viewId` on the panes IS the
// group (see `pane-views.ts`), so a split that lands in the layout is the
// same act as the chats being grouped. `groupIntoArrangement`, which used to
// file the pair into `dormantArrangements` as a second, independently-
// writable record of the same fact, is gone with it.
describe('pane-slice — views are the grouping fact (spec §8.2 "merging")', () => {
  it('a freshly added pane is its own view — nothing else carries its id', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-0', null)
    const a = store.getState().paneActions.addPane()!
    // Filled before the next one opens: an empty view is nothing to switch
    // back to, so `addPane` evaporates one rather than parking it.
    store.getState().paneActions.setPaneChat(a, 'chat-a', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-b', null)

    const { panes } = store.getState()
    expect(viewIdOf(panes[a])).not.toBe(viewIdOf(panes[b]))
    expect(viewIdOf(panes[a])).not.toBe(viewIdOf(panes[ROOT_PANE_ID]))
    // Three views, one on screen — the whole point of the tag.
    expect(new Set([a, b, ROOT_PANE_ID].map((id) => viewIdOf(panes[id]))).size).toBe(3)
  })

  it('a split INHERITS the view it was carved out of — that is the merge', () => {
    const store = makeStore()
    const target = store.getState().paneActions.addPane()!
    const merged = store.getState().paneActions.splitPane(target, 'horizontal')!

    const { panes } = store.getState()
    expect(viewIdOf(panes[merged])).toBe(viewIdOf(panes[target]))
  })

  it('a merged view is ONE Recents row carrying every chat in it', () => {
    const store = makeStore()
    const target = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(target, 'chat-1', null)
    const merged = store.getState().paneActions.splitPane(target, 'horizontal')!
    store.getState().paneActions.setPaneChat(merged, 'chat-2', null)

    const { panes, dormantArrangements, recentsOrder } = store.getState()
    const entries = deriveRecentsEntries(
      Object.values(panes),
      {},
      dormantArrangements,
      recentsOrder,
    )

    expect(entries).toHaveLength(1)
    expect([...entries[0].chatIds].sort()).toEqual(['chat-1', 'chat-2'])
    expect(entries[0].id).toBe(viewIdOf(panes[target]))
  })

  it('two separately-added panes are two rows, never one — a click never merges', () => {
    const store = makeStore()
    const a = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(a, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    const { panes, dormantArrangements, recentsOrder } = store.getState()
    const entries = deriveRecentsEntries(
      Object.values(panes),
      {},
      dormantArrangements,
      recentsOrder,
    )

    expect(entries).toHaveLength(2)
    expect(entries.map((e) => e.chatIds)).toEqual([['chat-1'], ['chat-2']])
  })

  // Zen's own rule, and the reason no code anywhere has to notice it: a group
  // of one and an ungrouped pane are the same thing, so a merged view losing
  // its second member is simply an ordinary view again.
  it('a view down to one pane behaves exactly like an independent one', () => {
    const store = makeStore()
    const target = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(target, 'chat-1', null)
    const merged = store.getState().paneActions.splitPane(target, 'horizontal')!
    store.getState().paneActions.setPaneChat(merged, 'chat-2', null)

    store.getState().paneActions.closePane(merged)

    const { panes, dormantArrangements, recentsOrder } = store.getState()
    expect(
      Object.values(panes).filter((p) => viewIdOf(p) === viewIdOf(panes[target])),
    ).toHaveLength(1)
    const live = deriveRecentsEntries(
      Object.values(panes),
      {},
      dormantArrangements,
      recentsOrder,
    ).find((e) => e.state === 'live')
    expect(live?.chatIds).toEqual(['chat-1'])
  })

  it('detachPaneToOwnView pulls a merged pane out into a view of its own', () => {
    const store = makeStore()
    const target = store.getState().paneActions.addPane()!
    const merged = store.getState().paneActions.splitPane(target, 'horizontal')!
    expect(viewIdOf(store.getState().panes[merged])).toBe(viewIdOf(store.getState().panes[target]))

    store.getState().paneActions.detachPaneToOwnView(merged)

    const { panes } = store.getState()
    expect(viewIdOf(panes[merged])).not.toBe(viewIdOf(panes[target]))
  })

  // The trap the implementation calls out: a pane split OFF of P carries
  // `viewId === P.id`, so detaching P by reusing its own id would leave the
  // two still grouped.
  it('detaching the pane a split was carved FROM really separates the two', () => {
    const store = makeStore()
    const target = store.getState().paneActions.addPane()!
    const merged = store.getState().paneActions.splitPane(target, 'horizontal')!

    store.getState().paneActions.detachPaneToOwnView(target)

    const { panes } = store.getState()
    expect(viewIdOf(panes[target])).not.toBe(viewIdOf(panes[merged]))
  })

  it('is a no-op for a pane that is already a view of its own', () => {
    const store = makeStore()
    const solo = store.getState().paneActions.addPane()!
    const before = store.getState().panes

    store.getState().paneActions.detachPaneToOwnView(solo)

    expect(store.getState().panes).toBe(before)
  })
})

describe('pane-slice — setPaneChat sheds stale arrangement membership (spec §8.2)', () => {
  it('a chat moving fresh into a NEW pane leaves every arrangement that remembered it, survivors kept as a set', () => {
    const store = makeStore()
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2', 'chat-3'])
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
    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual([
      'chat-1',
      'chat-3',
    ])
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
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2'])
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
    const { paneActions } = store.getState()
    // A single-chat entry, exactly as setPaneChat's own hotswap-away archive
    // creates (spec §8.4) when chat-2 replaces chat-1 in the same pane.
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')
    const [dormantRecord] = store.getState().dormantArrangements
    expect(dormantRecord).toEqual({
      id: expect.any(String),
      chatIds: ['chat-1'],
      state: 'dormant',
    })

    const before = store.getState().dormantArrangements
    // Restore it into its OWN, still-empty pane — the same write
    // `openAgentChat` performs — not swapped back over chat-2 (which would
    // just re-archive chat-2 instead, per §8.4, and confound this assertion).
    const other = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(other, 'chat-1', null)

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
    seedArrangement(store, 'set-other', ['chat-2', 'chat-3']) // unrelated set
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
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2'])

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)

    expect([...store.getState().dormantArrangements[0].chatIds].sort()).toEqual([
      'chat-1',
      'chat-2',
    ])
  })
})

// Task 23: spec §8.4 — "clicking a chat... does not take over the pane you
// were in — that arrangement is put down whole, into Recents... Nothing you
// click ever costs you what you were looking at." Every click-to-open
// surface (openAgentChat — used by the Chats sidebar's row click, the New
// Tab recent list, and a review-thread attribution click — plus ⌘N's
// AGENT_NEW_CHAT handler) funnels through this one write path, so the
// archive-on-swap guarantee lives here rather than re-derived at each site.
describe('pane-slice — setPaneChat archives an evicted chat (spec §8.4)', () => {
  it('swapping a pane onto a different chat archives the one it held as dormant', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')

    expect(store.getState().dormantArrangements).toEqual([
      { id: expect.any(String), chatIds: ['chat-1'], state: 'dormant' },
    ])
    // Nothing was lost from the pane itself either — it just shows the new chat.
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-2')
  })

  it('archives with a fresh id, never the pane id — the pane goes live again on the new chat', () => {
    const store = makeStoreWithWorking({})
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')

    // Reusing `paneId` here would collide with the live entry
    // `deriveRecentsEntries` derives for this same pane below.
    const [dormantRecord] = store.getState().dormantArrangements
    expect(dormantRecord.id).not.toBe(ROOT_PANE_ID)

    const entries = deriveRecentsEntries(Object.values(store.getState().panes), {}, [dormantRecord])
    expect(entries).toEqual([
      { id: dormantRecord.id, chatIds: ['chat-1'], state: 'dormant' },
      { id: ROOT_PANE_ID, chatIds: ['chat-2'], state: 'live' },
    ])
  })

  it('does not archive a WORKING chat being swapped out — its row lives on agentChats.working alone', () => {
    const store = makeStoreWithWorking({ 'chat-1': true })
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')

    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('does not double-archive a chat that already has its own dormant slot', () => {
    const store = makeStoreWithWorking({})
    const { paneActions } = store.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-3', 'runner-3') // archives chat-1 as dormant
    // Restore chat-1 into its OWN, still-empty pane (spec §5.6 keeps this the
    // SAME record, at its slot) — not swapped back over chat-3, which would
    // just archive chat-3 too and confound the length assertion below.
    const other = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(other, 'chat-1', null)
    expect(store.getState().dormantArrangements).toHaveLength(1)

    store.getState().paneActions.setPaneChat(other, 'chat-2', 'runner-2')

    // chat-1 already had a slot remembering it — swapping it back out must
    // not mint a second record alongside that one.
    expect(store.getState().dormantArrangements).toHaveLength(1)
    expect(store.getState().dormantArrangements[0].chatIds).toEqual(['chat-1'])
  })

  it('never evicts a chat that is already open somewhere — openAgentChat reveals instead of re-opening', () => {
    // The other half of §8.4: "a chat that is already up is gone TO, never
    // opened twice." openAgentChat's own dedup (reveal via setActivePane)
    // means setPaneChat is never called a second time for a chat that is
    // already live in some pane — exercised here at the real call path
    // (open-agent-chat.ts's openAgentChat), the one every reachable click
    // surface goes through today (new-tab-view.tsx, review-thread-item.tsx,
    // and — once chat rows resolve through rowsFromRepo's tree — the
    // sidebar).
    resetWindowPaneStoreForTests()
    const wsStore = createWorkspaceStore('ws-open-agent-chat-test')
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const paneBId = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    windowPaneStore.getState().paneActions.setPaneChat(paneBId!, 'chat-2', 'runner-2')

    openAgentChat(wsStore, 'ws-open-agent-chat-test', 'chat-1')

    const panesWithChat1 = Object.values(windowPaneStore.getState().panes).filter(
      (p) => p.chatId === 'chat-1',
    )
    expect(panesWithChat1).toHaveLength(1)
    // Revealed where it already lives, not costing pane B's chat-2.
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().panes[paneBId!]?.chatId).toBe('chat-2')
  })

  it('is a no-op (never throws) on a bare pane-slice store with no agentChats', () => {
    const actions = makeStore().getState().paneActions
    actions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    expect(() => actions.setPaneChat(ROOT_PANE_ID, 'chat-2', 'runner-2')).not.toThrow()
  })
})

describe('pane-slice — closePane splits the closing chat out of any SET it belongs to', () => {
  it("strips the closed chat from the set, leaving the survivor at the SET's own slot, and leaves the closed chat with no record of its own", () => {
    const store = makeStoreWithWorking({})
    // chat-1 is genuinely resident in ROOT, and some multi-chat entry
    // remembers it alongside chat-2.
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2'])
    const setId = store.getState().dormantArrangements[0].id

    // Closing chat-1's pane through the tab bar / pane-close keybinding —
    // never Recents' own × control, which closes every member's pane at
    // once — must not leave chat-1 riding along inside the set forever
    // (the bug this pins: `resolveState` reads an entry live off ANY
    // member still showing, so a stale chat-1 would draw as "live" long
    // after its own pane was gone).
    store.getState().paneActions.closePane(ROOT_PANE_ID)

    const entries = store.getState().dormantArrangements
    // The survivor keeps the SET's own slot (spec §5.6), even reduced to
    // one member — and that is the ONLY entry left. A close is final now:
    // chat-1 gets no record of its own to ride along in Recents with.
    expect(entries).toHaveLength(1)
    expect(entries[0].id).toBe(setId)
    expect(entries[0].chatIds).toEqual(['chat-2'])
  })

  it('does not remember the split-out chat if the daemon is still working it', () => {
    const store = makeStoreWithWorking({ 'chat-1': true })
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedArrangement(store, 'set-1', ['chat-1', 'chat-2'])
    const setId = store.getState().dormantArrangements[0].id

    store.getState().paneActions.closePane(ROOT_PANE_ID)

    // Still split out of the set (its pane view genuinely ended)...
    const entries = store.getState().dormantArrangements
    expect(entries).toHaveLength(1)
    expect(entries[0].id).toBe(setId)
    expect(entries[0].chatIds).toEqual(['chat-2'])
    // ...but no dormant record for chat-1 itself: it comes back as a
    // working row off `agentChats.working` alone (spec §5.5).
  })

  it('re-closing a chat that already had its own dormant slot (from an earlier hotswap-away) purges that record too', () => {
    const store = makeStoreWithWorking({})
    const { paneActions } = store.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-3', 'runner-3') // archives chat-1 as dormant
    // Reopened into its OWN, still-empty pane — not swapped back over
    // chat-3, which would just archive chat-3 too.
    const other = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    paneActions.setPaneChat(other, 'chat-1', null)
    expect(store.getState().dormantArrangements).toHaveLength(1)

    store.getState().paneActions.closePane(other)

    // A close is final now — no record survives it, stale or otherwise.
    expect(store.getState().dormantArrangements).toEqual([])
  })
})

// Spec §8.1: "above / below a Recents entry → it moves to that slot" — the
// persisted order `drop-actions.ts`'s `reorderRecentsEntry` writes into and
// `recents-entries.ts`'s `deriveRecentsEntries` sorts by.
describe('pane-slice — reorderRecentsEntry (spec §8.1)', () => {
  it('seeds the ledger from the given natural order, then moves the source before the target', () => {
    const store = makeStore()

    store.getState().paneActions.reorderRecentsEntry('c', 'a', 'before', ['a', 'b', 'c'])

    expect(store.getState().recentsOrder).toEqual(['c', 'a', 'b'])
  })

  it('moves the source after the target', () => {
    const store = makeStore()

    store.getState().paneActions.reorderRecentsEntry('a', 'c', 'after', ['a', 'b', 'c'])

    expect(store.getState().recentsOrder).toEqual(['b', 'c', 'a'])
  })

  it('a second reorder only touches the moved id, leaving the rest of the ledger alone', () => {
    const store = makeStore()
    store.getState().paneActions.reorderRecentsEntry('c', 'a', 'before', ['a', 'b', 'c'])
    expect(store.getState().recentsOrder).toEqual(['c', 'a', 'b'])

    store.getState().paneActions.reorderRecentsEntry('b', 'c', 'after', ['c', 'a', 'b'])

    expect(store.getState().recentsOrder).toEqual(['c', 'b', 'a'])
  })

  it('never disturbs an id some OTHER project already placed — only ever appends unseen ids', () => {
    const store = makeStore()
    // 'x'/'y' stand in for another project's already-reordered entries —
    // this drag's own `naturalOrder` (['a', 'b']) knows nothing about them.
    store.getState().paneActions.reorderRecentsEntry('y', 'x', 'after', ['x', 'y'])
    expect(store.getState().recentsOrder).toEqual(['x', 'y'])

    store.getState().paneActions.reorderRecentsEntry('b', 'a', 'before', ['a', 'b'])

    // x/y's relative order survives untouched; a/b are freshly seeded and
    // appended before the move is applied.
    expect(store.getState().recentsOrder).toEqual(['x', 'y', 'b', 'a'])
  })

  it('a target not yet in the ledger (and not in naturalOrder either) appends the source at the end', () => {
    const store = makeStore()

    store.getState().paneActions.reorderRecentsEntry('a', 'ghost', 'before', ['a'])

    expect(store.getState().recentsOrder).toEqual(['a'])
  })
})

/**
 * VIEW ACTIVATION — the law the whole view model exists to enforce: exactly
 * one view occupies the content area, and every other open view is whole,
 * reachable and doing nothing.
 *
 * `rootLayout` IS the showing view's tree, so "only the active view renders"
 * needs no filter in the render path — it is a property of the data the
 * renderer already walks. These assert that property directly.
 */
describe('pane-slice — one view on screen at a time', () => {
  it('the showing tree only ever holds ONE view id', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const merged = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(merged, 'chat-2', null)
    const other = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(other, 'chat-3', null)
    store.getState().paneActions.activateView(ROOT_PANE_ID)

    const { rootLayout, panes, activeViewId } = store.getState()
    const viewIds = new Set(getAllLeafIds(rootLayout).map((id) => viewIdOf(panes[id])))
    expect([...viewIds]).toEqual([activeViewId])
  })

  it('activating a view parks the one that was showing, whole', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    store.getState().paneActions.activateView(ROOT_PANE_ID)

    expect(store.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(Object.keys(store.getState().parkedViews)).toEqual([viewIdOf(store.getState().panes[b])])
    // Parked is not closed — the chat is untouched.
    expect(store.getState().panes[b].chatId).toBe('chat-2')
  })

  it('is a no-op for the view already showing, and for one nothing parked', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const before = store.getState()

    store.getState().paneActions.activateView(ROOT_PANE_ID)
    store.getState().paneActions.activateView('a-view-that-never-existed')

    expect(store.getState().rootLayout).toBe(before.rootLayout)
    expect(store.getState().activeViewId).toBe(before.activeViewId)
  })

  it('focusing a pane in a parked view brings that whole view over', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([b])

    // Every "go to that chat" gesture in the app routes through this one
    // action, which is why none of them needs switching code of its own.
    store.getState().paneActions.setActivePane(ROOT_PANE_ID)

    expect(store.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
  })

  it('a switched-to view restores the pane that was last focused IN IT', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const mate = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(mate, 'chat-2', null)
    store.getState().paneActions.setActivePane(mate)

    const away = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(away, 'chat-3', null)
    store.getState().paneActions.activateView(ROOT_PANE_ID)

    expect(store.getState().activePaneId).toBe(mate)
  })
})

/**
 * `mergePaneIntoView` — MOVING an already-open pane into another view.
 *
 * The action that makes a drag-created split possible for a chat that is
 * already up. `splitPane` alone mints an EMPTY pane, so a caller holding a
 * chat that already has one could only duplicate it (against §8.2's "it never
 * opens twice") or refuse the split. Refusing is what shipped, and once every
 * chat got a view of its own that made a split unreachable for anything the
 * user had ever opened.
 *
 * A MOVE, deliberately: none of `closePane`'s teardown runs, so the pane
 * carries its live chat, runner and editor tabs across intact.
 */
describe('pane-slice — mergePaneIntoView', () => {
  it('re-homes a parked view’s pane as a split of the showing one, in ONE view', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)
    // `b`'s view is showing; ROOT's is parked.
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([b])

    store.getState().paneActions.mergePaneIntoView(ROOT_PANE_ID, b, 'horizontal', 'after')

    expect(getAllLeafIds(store.getState().rootLayout).sort()).toEqual([ROOT_PANE_ID, b].sort())
    expect(viewIdOf(store.getState().panes[ROOT_PANE_ID])).toBe(viewIdOf(store.getState().panes[b]))
    // The view it left held nothing else — gone, not parked empty.
    expect(store.getState().parkedViews).toEqual({})
  })

  it('carries the pane’s own live state across — no close, no reopen', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    store.getState().paneActions.mergePaneIntoView(ROOT_PANE_ID, b, 'vertical', 'before')

    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().panes[ROOT_PANE_ID].runnerId).toBe('runner-1')
    // Nothing was remembered as closed — it never closed.
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('honours placement — "before" puts the arriving pane first', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    store.getState().paneActions.mergePaneIntoView(ROOT_PANE_ID, b, 'horizontal', 'before')

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID, b])
  })

  it('leaves a multi-pane source view standing, minus the pane that left', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const mate = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(mate, 'chat-2', null)
    const target = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(target, 'chat-3', null)
    const sourceView = viewIdOf(store.getState().panes[ROOT_PANE_ID])

    store.getState().paneActions.mergePaneIntoView(mate, target, 'horizontal', 'after')

    expect(getAllLeafIds(store.getState().parkedViews[sourceView])).toEqual([ROOT_PANE_ID])
    expect(getAllLeafIds(store.getState().rootLayout).sort()).toEqual([mate, target].sort())
    expect(viewIdOf(store.getState().panes[mate])).toBe(viewIdOf(store.getState().panes[target]))
  })

  it('brings the target’s view over when the pane leaving emptied the screen', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const parked = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(parked, 'chat-2', null)
    store.getState().paneActions.activateView(ROOT_PANE_ID) // ROOT showing, `parked` parked

    // ROOT is the whole showing tree, and it is being merged into the parked one.
    store.getState().paneActions.mergePaneIntoView(ROOT_PANE_ID, parked, 'horizontal', 'after')

    expect(store.getState().parkedViews).toEqual({})
    expect(getAllLeafIds(store.getState().rootLayout).sort()).toEqual([ROOT_PANE_ID, parked].sort())
    expect(store.getState().activeViewId).toBe(viewIdOf(store.getState().panes[parked]))
  })

  it('collapses the empty stage it landed beside — a fallback is not a split partner', () => {
    const store = makeStore()
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-1', null)
    store.getState().paneActions.activateView(ROOT_PANE_ID) // the empty stage is showing

    store.getState().paneActions.mergePaneIntoView(b, ROOT_PANE_ID, 'horizontal', 'after')

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([b])
    expect(store.getState().panes[ROOT_PANE_ID]).toBeUndefined()
  })

  it('is a no-op for a pane merged into itself, or into one that does not exist', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const before = store.getState().rootLayout

    store
      .getState()
      .paneActions.mergePaneIntoView(ROOT_PANE_ID, ROOT_PANE_ID, 'horizontal', 'after')
    store
      .getState()
      .paneActions.mergePaneIntoView(ROOT_PANE_ID, 'no-such-pane', 'horizontal', 'after')

    expect(store.getState().rootLayout).toBe(before)
  })
})

/**
 * Closing. The teardown semantics are unchanged and deliberately so — a close
 * still stops the vendor CLI and evicts the workspace store via
 * `releaseClosedChat` — but a view of any size must now take ALL of its panes
 * with it, and closing what is on screen must reveal what is behind it rather
 * than dropping the user on an empty stage.
 */
describe('pane-slice — closing a view', () => {
  it('closeView ends every pane in the view, not just the one named', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)
    const c = store.getState().paneActions.splitPane(b, 'vertical')!
    store.getState().paneActions.setPaneChat(c, 'chat-3', null)
    const view = viewIdOf(store.getState().panes[ROOT_PANE_ID])

    store.getState().paneActions.closeView(view)

    const live = Object.values(store.getState().panes).filter((p) => p.chatId !== null)
    expect(live).toEqual([])
    expect(store.getState().panes[b]).toBeUndefined()
    expect(store.getState().panes[c]).toBeUndefined()
    // A close is final on every member now — none of the three chats leaves
    // a dormant Recents row behind for a second, separate dismiss.
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('closing a view never touches a sibling view', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const other = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(other, 'chat-2', null)

    store.getState().paneActions.closeView(viewIdOf(store.getState().panes[other]))

    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
  })

  it('closing the showing view REVEALS the one behind it, not the empty stage', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    store.getState().paneActions.closePane(b)

    expect(store.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().parkedViews).toEqual({})
  })

  it('closing the LAST view leaves the empty stage — that one really is a fallback', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)

    store.getState().paneActions.closePane(ROOT_PANE_ID)

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
    expect(store.getState().parkedViews).toEqual({})
  })

  it('closing one member of a merged view leaves the rest showing and grouped', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const b = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(b, 'chat-2', null)

    store.getState().paneActions.closePane(b)

    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([ROOT_PANE_ID])
    expect(store.getState().activeViewId).toBe(viewIdOf(store.getState().panes[ROOT_PANE_ID]))
  })
})

/**
 * A view that is off screen is still a real arrangement that can be grown —
 * that is what makes an inactive view's Recents row a valid drop target
 * (spec §8.1's "into that view, opened"). Before views owned their own trees
 * this could not work at all: the split machinery looked the pane up in
 * `rootLayout` only, found nothing, and silently did nothing.
 */
describe('pane-slice — editing a view that is not on screen', () => {
  it('splitting a parked pane grows THAT view, and leaves the screen alone', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const showing = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(showing, 'chat-2', null)

    const grown = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(grown, 'chat-3', null)

    // The parked view really grew...
    expect(getAllLeafIds(store.getState().parkedViews[ROOT_PANE_ID]).sort()).toEqual(
      [ROOT_PANE_ID, grown].sort(),
    )
    expect(viewIdOf(store.getState().panes[grown])).toBe(ROOT_PANE_ID)
    // ...without yanking the screen, and without pointing focus at a pane
    // nothing renders.
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([showing])
    expect(store.getState().activePaneId).toBe(showing)
  })

  it('closing the last pane of a parked view drops the view outright', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const showing = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(showing, 'chat-2', null)

    store.getState().paneActions.closePane(ROOT_PANE_ID)

    expect(store.getState().parkedViews).toEqual({})
    expect(store.getState().panes[ROOT_PANE_ID]).toBeUndefined()
    // The screen is untouched throughout.
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([showing])
    expect(store.getState().activePaneId).toBe(showing)
  })

  it('a parked pane emptied by an eviction takes its dead view with it', () => {
    const store = makeStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
    const showing = store.getState().paneActions.addPane()!
    store.getState().paneActions.setPaneChat(showing, 'chat-2', null)

    // `followRunner`'s eviction, landing on a pane in a view nobody is
    // looking at.
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)

    expect(store.getState().parkedViews).toEqual({})
    expect(store.getState().panes[ROOT_PANE_ID]).toBeUndefined()
    expect(getAllLeafIds(store.getState().rootLayout)).toEqual([showing])
  })
})
