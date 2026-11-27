import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { workspaceRef } from '@/lib/api/documents'

const rendersSomething = (paneId: string): boolean => {
  const pane = windowPaneStore.getState().panes[paneId]
  if (!pane?.activeEditorTabId) return false
  return pane.editorTabIds.includes(pane.activeEditorTabId)
}

describe('jump navigation — pane membership invariant', () => {
  it('renders the file when it is already a tab in the active pane', () => {
    resetWindowPaneStoreForTests()
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('w1', '/a.ts'),
      name: 'a.ts',
    })

    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, id)

    expect(rendersSomething(ROOT_PANE_ID)).toBe(true)
  })

  it('refuses to activate a buffer the pane does not hold instead of blanking the pane', () => {
    resetWindowPaneStoreForTests()
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('w1', '/a.ts'),
      name: 'a.ts',
    })

    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)
    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, id)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).not.toBe(id)
    expect(rendersSomething(ROOT_PANE_ID)).toBe(false)
  })

  it('reopening a closed file seats and activates it again', () => {
    resetWindowPaneStoreForTests()
    const spec = {
      type: 'editor' as const,
      doc: workspaceRef('w1', '/a.ts'),
      name: 'a.ts',
    }
    const id = windowPaneStore.getState().bufferActions.openContent(spec)
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)
    expect(windowPaneStore.getState().buffers).toEqual([])

    windowPaneStore.getState().bufferActions.openContent(spec)

    expect(rendersSomething(ROOT_PANE_ID)).toBe(true)
  })
})
