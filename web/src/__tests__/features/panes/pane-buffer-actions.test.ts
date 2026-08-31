import { beforeEach, describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

describe('pane buffer actions', () => {
  beforeEach(() => {
    // Task 26: panes/buffers are one window-level singleton now, never
    // destroyed — reset it to a pristine store (mirroring
    // createWindowPaneStore's own defaults) before each test.
    resetWindowPaneStoreForTests()
  })

  it('adds a missing buffer to an existing pane', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')
    // addEditorTabToPane only registers a REFERENCE (see the comment on
    // ensureBufferInPane) — the buffer itself must already exist.
    windowPaneStore.setState((state) => {
      state.buffers.push({
        id: 'buffer-a',
        type: 'editor',
        path: '/buffer-a.ts',
        name: 'buffer-a.ts',
        content: '',
        savedContent: '',
        isDirty: false,
        isVirtual: false,
        tokens: [],
        isPinned: false,
        isPreview: false,
        workspaceId: 'test-ws',
      })
      return state
    })

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toEqual(['buffer-a'])
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activates existing buffers without duplicating them', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'test-ws',
    })
    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'test-ws',
    })

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['buffer-a', 'buffer-b'])
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('buffer-a')
  })

  it('does nothing when the buffer does not exist anywhere (nothing to reference)', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane(ROOT_PANE_ID, 'ghost-buffer')).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toEqual([])
  })

  it('returns null for missing panes', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane('missing-pane', 'buffer-a')).toBeNull()
  })
})
