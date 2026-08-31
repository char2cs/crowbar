import { beforeEach, describe, expect, it } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  createWindowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'

// Task 26 moved panes/buffers out of the per-workspace store registry into one
// window-level store, and Task 1 renamed the pane's tab list `bufferIds` ->
// `editorTabIds` (a chat is no longer one of them at all: it is the pane's own
// `chatId` field). This suite used to drive `createWorkspaceStore('test-ws')`
// and the pre-Task-1 action names, so every case in it threw
// `addBufferToPane is not a function` — migrated here onto the real store and
// the real action names.
//
// A fresh store per test rather than the module singleton: these cases split,
// close and merge the layout, and the singleton is shared with every other
// suite in the run.

function makeStore(): WindowPaneStore {
  return createWindowPaneStore()
}

let store: WindowPaneStore

const paneActions = () => store.getState().paneActions
const panes = () => store.getState().panes

/** A real editor buffer plus its tab in `paneId`. The buffer half matters:
 *  `setEditorTabPreview`/`setEditorTabPinned` scope-and-mutate the TAB'S OWN
 *  `isPreview`/`isPinned` (there are no pane-level `previewBufferId`/
 *  `pinnedBufferIds` fields — Task 2's ruling), so a tab id with no buffer
 *  behind it has nothing to mark. */
function openTab(paneId: string, id: string): void {
  const buffer: EditorContent = {
    id,
    type: 'editor',
    path: `src/${id}.ts`,
    name: `${id}.ts`,
    workspaceId: 'test-ws',
    content: '',
    savedContent: '',
    isDirty: false,
    isVirtual: false,
    tokens: [],
  }
  store.setState((state) => {
    if (!state.buffers.some((b) => b.id === id)) state.buffers.push(buffer)
    return state
  })
  paneActions().addEditorTabToPane(paneId, buffer)
}

const bufferById = (id: string) => store.getState().buffers.find((b) => b.id === id)

beforeEach(() => {
  store = makeStore()
})

describe('pane-store bottom pane integration', () => {
  it('moves editor tabs between the root pane and the bottom pane', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    paneActions().moveEditorTabToPane('buffer-a', ROOT_PANE_ID, BOTTOM_PANE_ID)

    // A pane emptied by a move is simply EMPTY now. The old "never tab-less,
    // reseed a newTab placeholder" contract went with Task 1: a pane with zero
    // editorTabIds renders the New Tab stage for free (pane-container.tsx), so
    // there is no placeholder buffer left to assert.
    expect(panes()[ROOT_PANE_ID]?.editorTabIds).toEqual([])
    expect(panes()[ROOT_PANE_ID]?.editorOpen).toBe(false)
    expect(panes()[BOTTOM_PANE_ID]?.editorTabIds).toEqual(['buffer-a'])
    expect(panes()[BOTTOM_PANE_ID]?.activeEditorTabId).toBe('buffer-a')

    paneActions().moveEditorTabToPane('buffer-a', BOTTOM_PANE_ID, ROOT_PANE_ID)

    expect(panes()[ROOT_PANE_ID]?.editorTabIds).toEqual(['buffer-a'])
    expect(panes()[ROOT_PANE_ID]?.activeEditorTabId).toBe('buffer-a')
    expect(panes()[BOTTOM_PANE_ID]?.editorTabIds).toEqual([])
  })

  it('can split the bottom root like any other pane tree', () => {
    openTab(BOTTOM_PANE_ID, 'buffer-a')
    const newPaneId = paneActions().splitPane(BOTTOM_PANE_ID, 'horizontal')

    expect(newPaneId).not.toBeNull()
    expect(paneActions().getAllPaneGroups().length).toBeGreaterThanOrEqual(2)
    expect(panes()[BOTTOM_PANE_ID]?.editorTabIds).toEqual(['buffer-a'])
  })

  it('preserves an empty source pane when moving the only tab into a new split', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const newPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    if (!newPaneId) return

    paneActions().moveEditorTabToPane('buffer-a', ROOT_PANE_ID, newPaneId)

    expect(paneActions().getAllPaneGroups().length).toBeGreaterThanOrEqual(2)
    expect(panes()[ROOT_PANE_ID]).toBeDefined() // the source survives, empty
    expect(panes()[ROOT_PANE_ID]?.editorTabIds).toEqual([])
    expect(panes()[newPaneId]?.editorTabIds).toEqual(['buffer-a'])
  })

  it('falls back to the most recently active remaining pane when closing the active pane', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    const bottomPaneId = paneActions().splitPane(rightPaneId, 'vertical')
    expect(bottomPaneId).not.toBeNull()
    if (!bottomPaneId) return

    paneActions().setActivePane(ROOT_PANE_ID)
    paneActions().setActivePane(rightPaneId)
    paneActions().setActivePane(bottomPaneId)
    openTab(bottomPaneId, 'buffer-b')
    paneActions().closePane(bottomPaneId)

    const newActiveId = store.getState().activePaneId
    expect([ROOT_PANE_ID, rightPaneId]).toContain(newActiveId)
    // The closed pane's tabs merge into the survivor rather than being dropped.
    expect(panes()[newActiveId]?.editorTabIds).toContain('buffer-b')
  })

  it('merges editor tabs into the fallback pane when closing an inactive pane', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    openTab(rightPaneId, 'buffer-b')
    paneActions().setActivePane(ROOT_PANE_ID)
    paneActions().closePane(rightPaneId)

    const rootPane = paneActions().getPaneById(ROOT_PANE_ID)
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(rootPane?.editorTabIds).toContain('buffer-a')
    expect(rootPane?.editorTabIds).toContain('buffer-b')
  })

  it('activates a tab and its pane as a single operation', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    openTab(rightPaneId, 'buffer-b')
    paneActions().setActivePane(ROOT_PANE_ID)
    paneActions().activateEditorTabInPane(rightPaneId, 'buffer-b')

    expect(store.getState().activePaneId).toBe(rightPaneId)
    expect(store.getState().mostRecentActivePaneIds[0]).toBe(rightPaneId)
    expect(paneActions().getPaneById(rightPaneId)?.activeEditorTabId).toBe('buffer-b')
  })

  it('routes pane-local tab cycling through pane activation metadata', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    openTab(rightPaneId, 'buffer-b')
    openTab(rightPaneId, 'buffer-c')
    paneActions().setActivePane(ROOT_PANE_ID)
    paneActions().activateEditorTabInPane(rightPaneId, 'buffer-b')

    // switchToNext/PreviousEditorTab take the pane explicitly now (the old
    // switchToNextBufferInPane read the active pane off the store).
    paneActions().switchToNextEditorTab(rightPaneId)

    expect(store.getState().activePaneId).toBe(rightPaneId)
    expect(paneActions().getPaneById(rightPaneId)?.activeEditorTabId).toBe('buffer-c')

    paneActions().switchToPreviousEditorTab(rightPaneId)

    expect(store.getState().activePaneId).toBe(rightPaneId)
    expect(paneActions().getPaneById(rightPaneId)?.activeEditorTabId).toBe('buffer-b')
  })

  it('tracks preview and pinned metadata on the tabs a pane holds', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    openTab(ROOT_PANE_ID, 'buffer-b')

    paneActions().setEditorTabPreview(ROOT_PANE_ID, 'buffer-a')

    // Preview is a single slot per pane: marking one clears every other tab
    // that pane holds.
    expect(bufferById('buffer-a')?.isPreview).toBe(true)
    expect(bufferById('buffer-b')?.isPreview).toBe(false)

    paneActions().setEditorTabPinned(ROOT_PANE_ID, 'buffer-a', true)
    expect(bufferById('buffer-a')?.isPinned).toBe(true)

    paneActions().setEditorTabPinned(ROOT_PANE_ID, 'buffer-a', false)
    expect(bufferById('buffer-a')?.isPinned).toBe(false)
  })

  it('refuses to mark a tab the pane does not hold', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    if (!rightPaneId) return

    paneActions().setEditorTabPreview(rightPaneId, 'buffer-a')

    expect(bufferById('buffer-a')?.isPreview).toBeUndefined()
  })

  it('clears preview metadata wherever a tab is promoted', () => {
    openTab(ROOT_PANE_ID, 'buffer-a')
    const splitPaneId = paneActions().splitPane(ROOT_PANE_ID, 'horizontal')
    expect(splitPaneId).not.toBeNull()
    if (!splitPaneId) return

    openTab(splitPaneId, 'buffer-b')
    paneActions().setEditorTabPreview(ROOT_PANE_ID, 'buffer-a')
    paneActions().setEditorTabPreview(splitPaneId, 'buffer-b')
    expect(bufferById('buffer-a')?.isPreview).toBe(true)
    expect(bufferById('buffer-b')?.isPreview).toBe(true)

    paneActions().clearEditorTabPreviewEverywhere()

    expect(bufferById('buffer-a')?.isPreview).toBe(false)
    expect(bufferById('buffer-b')?.isPreview).toBe(false)
  })
})
