import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { workspaceRef } from '@/lib/api/documents'

// A pane renders only an active buffer among its own `editorTabIds` (see pane-container.tsx);
// `activateEditorTabInPane` refuses a non-member, so jump navigation must never use it for a
// buffer the target pane does not already hold.
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

    // Detach the tab while the buffer stays alive, the state jump navigation lands in after the
    // file's tab was closed; activateEditorTabInPane must still refuse to point the pane at `id`.
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)
    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, id)

    // The pane never points at something it does not hold (a tab strip with nothing rendered).
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
    // The pane let go of its last reference: the buffer is gone.
    expect(windowPaneStore.getState().buffers).toEqual([])

    windowPaneStore.getState().bufferActions.openContent(spec)

    expect(rendersSomething(ROOT_PANE_ID)).toBe(true)
  })
})
