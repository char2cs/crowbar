import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { TabCloseConfirm } from '@/features/tabs/components/tab-close-confirm'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import type { EditorContent } from '@/features/panes/types/pane-content'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

function editorTab(id: string, isDirty: boolean): EditorContent {
  return {
    id,
    type: 'editor',
    authority: 'workspace',
    path: `/repo/${id}`,
    name: id,
    isDirty,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    workspaceId: 'ws-a',
  }
}

/** Two chats split side by side; the left one holds a dirty README beside another tab. */
function twoPanesWithDirtyReadme() {
  const { paneActions } = windowPaneStore.getState()
  paneActions.openChat('c1', { workspaceId: 'ws-a' })
  paneActions.dropChatOnPane('c2', ROOT_PANE_ID, 'right', 'ws-b')
  const right = chatPaneIndex(windowPaneStore.getState().panes).get('c2') ?? ''
  windowPaneStore.setState((s) => {
    s.buffers = [editorTab('README.md', true), editorTab('notes.md', false)]
    const left = s.panes[ROOT_PANE_ID]
    left.editorTabIds = ['README.md', 'notes.md']
    left.activeEditorTabId = 'README.md'
    left.editorOpen = true
    return s
  })
  return { left: ROOT_PANE_ID, right }
}

beforeEach(() => resetWindowPaneStoreForTests())
afterEach(() => cleanup())

// Every pane's tab strip mounts a confirm; the one asked must be the only one to answer.
describe('TabCloseConfirm', () => {
  it('asks once, and Discard closes the tab in its own pane and selects its neighbour', () => {
    const { left, right } = twoPanesWithDirtyReadme()
    render(
      <>
        <TabCloseConfirm paneId={left} />
        <TabCloseConfirm paneId={right} />
      </>,
    )

    act(() =>
      windowPaneStore
        .getState()
        .bufferActions.setPendingClose({ bufferId: 'README.md', paneId: left }),
    )
    const discard = screen.getAllByRole('button', { name: 'Discard' })
    expect(discard).toHaveLength(1)
    act(() => fireEvent.click(discard[0]))

    const state = windowPaneStore.getState()
    expect(state.panes[left].editorTabIds).toEqual(['notes.md'])
    expect(state.panes[left].activeEditorTabId).toBe('notes.md')
    expect(state.buffers.map((b) => b.id)).toEqual(['notes.md'])
    expect(state.pendingClose).toBeNull()
    expect(screen.queryByRole('button', { name: 'Discard' })).toBeNull()
  })

  it('Cancel keeps the dirty tab open', () => {
    const { left } = twoPanesWithDirtyReadme()
    render(<TabCloseConfirm paneId={left} />)

    act(() =>
      windowPaneStore
        .getState()
        .bufferActions.setPendingClose({ bufferId: 'README.md', paneId: left }),
    )
    act(() => fireEvent.click(screen.getByRole('button', { name: 'Cancel' })))

    expect(windowPaneStore.getState().panes[left].editorTabIds).toEqual(['README.md', 'notes.md'])
    expect(windowPaneStore.getState().pendingClose).toBeNull()
  })
})
