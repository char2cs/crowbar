import { beforeEach, describe, expect, it } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  createWindowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'

function makeStore(): WindowPaneStore {
  return createWindowPaneStore()
}

let store: WindowPaneStore

const paneActions = () => store.getState().paneActions
const panes = () => store.getState().panes

function openTab(paneId: string, id: string): void {
  const buffer: EditorContent = {
    id,
    type: 'editor',
    authority: 'workspace',
    path: `src/${id}.ts`,
    name: `${id}.ts`,
    workspaceId: 'test-ws',
    isDirty: false,
    isVirtual: false,
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
    expect(panes()[ROOT_PANE_ID]).toBeDefined()
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
