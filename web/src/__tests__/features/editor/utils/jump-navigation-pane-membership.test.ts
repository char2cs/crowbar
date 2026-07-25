import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * A pane renders `paneBuffers.find((b) => b.id === pane.activeBufferId)`, where
 * `paneBuffers` is derived from `pane.bufferIds` (see pane-container.tsx). So a
 * pane whose `activeBufferId` names a buffer absent from its own `bufferIds`
 * resolves the active buffer to `null` and renders NOTHING — a blank editor.
 *
 * `activatePaneBuffer` sets `activeBufferId` without touching `bufferIds`, which
 * is why jump navigation must never call it for a buffer the target pane does
 * not already hold. This is the invariant every navigation path has to satisfy.
 */
const rendersSomething = (
  store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
): boolean => {
  const pane = store.getState().panes[paneId]
  if (!pane?.activeBufferId) return false
  return pane.bufferIds.includes(pane.activeBufferId)
}

describe('jump navigation — pane membership invariant', () => {
  it('renders the file when it is already a tab in the active pane', () => {
    const store = createWorkspaceStore('w1')
    const id = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: 'a' })

    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, id)

    expect(rendersSomething(store, ROOT_PANE_ID)).toBe(true)
  })

  it('refuses to activate a buffer the pane does not hold instead of blanking the pane', () => {
    const store = createWorkspaceStore('w1')
    const id = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: 'a' })

    // Detach the tab from the pane while the buffer itself stays alive — exactly
    // the state jump navigation lands in after the file's tab was closed.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, id, true)
    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, id)

    // `activatePaneBuffer` used to write the id straight through, leaving the
    // pane pointing at something it could not render (a blank editor under a
    // populated tab strip). It now refuses a non-member, so the pane goes on
    // showing whatever it legitimately holds — callers must ATTACH first.
    expect(store.getState().panes[ROOT_PANE_ID]?.activeBufferId).not.toBe(id)
    expect(rendersSomething(store, ROOT_PANE_ID)).toBe(true)
  })

  it('addBufferToPane restores renderability for a detached buffer', () => {
    const store = createWorkspaceStore('w1')
    const id = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: 'a' })
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, id, true)

    // The fix: attach before activating.
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, id, true)

    expect(rendersSomething(store, ROOT_PANE_ID)).toBe(true)
  })
})
