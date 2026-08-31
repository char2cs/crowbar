import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

/**
 * A pane renders `paneBuffers.find((b) => b.id === pane.activeEditorTabId)`,
 * where `paneBuffers` is derived from `pane.editorTabIds` (see
 * pane-container.tsx). So a pane whose `activeEditorTabId` names a buffer
 * absent from its own `editorTabIds` resolves the active buffer to `null`
 * and renders NOTHING — a blank editor.
 *
 * `activateEditorTabInPane` refuses to set `activeEditorTabId` for a tab
 * outside `editorTabIds`, which is why jump navigation must never call it
 * for a buffer the target pane does not already hold. This is the
 * invariant every navigation path has to satisfy. Task 26: panes/buffers
 * are window-level now — read `windowPaneStore`, not a per-workspace store.
 */
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
      path: '/a.ts',
      name: 'a.ts',
      content: 'a',
      workspaceId: 'w1',
    })

    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, id)

    expect(rendersSomething(ROOT_PANE_ID)).toBe(true)
  })

  it('refuses to activate a buffer the pane does not hold instead of blanking the pane', () => {
    resetWindowPaneStoreForTests()
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: 'a',
      workspaceId: 'w1',
    })

    // Detach the tab from the pane while the buffer itself stays alive — exactly
    // the state jump navigation lands in after the file's tab was closed.
    // Removing the sole tab correctly leaves the pane with none (its own
    // empty-stage fallback, no more "reseed with a New Tab placeholder" —
    // that concept was retired) — activeEditorTabId is null here already;
    // the invariant under test is that activateEditorTabInPane refuses to
    // point it at `id` anyway, since the pane still doesn't hold it.
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)
    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, id)

    // `activateEditorTabInPane` refuses a non-member: it never lets the pane
    // point at something it does not hold (a populated tab strip with
    // nothing rendered) — the old failure mode this invariant guards
    // against.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).not.toBe(id)
    expect(rendersSomething(ROOT_PANE_ID)).toBe(false)
  })

  it('addEditorTabToPane restores renderability for a detached buffer', () => {
    resetWindowPaneStoreForTests()
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: 'a',
      workspaceId: 'w1',
    })
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)

    // The fix: attach before activating. addEditorTabToPane always activates
    // the tab it adds (see pane-slice.ts), so no separate activate call.
    const buffer = windowPaneStore.getState().buffers.find((b) => b.id === id)!
    windowPaneStore.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, buffer)

    expect(rendersSomething(ROOT_PANE_ID)).toBe(true)
  })
})
