import { fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { ChatOnlyPaneHeader } from '@/features/tabs/components/chat-only-pane-header'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup } from '@/features/panes/types/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { openBranchReviewMock } = vi.hoisted(() => ({ openBranchReviewMock: vi.fn() }))
vi.mock('@/features/panes/utils/pane-command-actions', () => ({
  openBranchReviewForActiveWorkspace: openBranchReviewMock,
}))

function makeChat(overrides: Partial<AgentChat> = {}): AgentChat {
  return {
    id: 'chat-1',
    workspaceId: 'w1',
    title: 'My Chat',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    createdAt: new Date().toISOString(),
    parentId: '',
    ...overrides,
  } as AgentChat
}

function makePane(overrides: Partial<PaneGroup> = {}): PaneGroup {
  return {
    id: ROOT_PANE_ID,
    type: 'group',
    chatId: 'chat-1',
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    ...overrides,
  }
}

function renderHeader(pane: PaneGroup, wsId: string | null = 'w1') {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [makeChat()] },
  }))
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.panes[pane.id] = pane
    return s
  })
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatOnlyPaneHeader, { pane, wsId }),
    ),
  )
}

// TabBar's stand-in for a pane that holds a chat and no editor tabs at all —
// the whole IDE sector disappears, and this row takes over TabBar's own
// window-chrome duties (drag region) plus its right-pinned actions.
describe('ChatOnlyPaneHeader', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('carries the window drag region, same as TabBar’s own row', () => {
    renderHeader(makePane())
    const row = screen.getByTestId('pane-top-row')
    expect(row).toHaveAttribute('data-tauri-drag-region')
  })

  it("renders the chat's own identity header", () => {
    renderHeader(makePane())
    expect(screen.getByTestId('chat-branch-header')).toHaveTextContent('My Chat')
  })

  it('the branch-review shortcut opens branch review for the active workspace', () => {
    renderHeader(makePane())
    fireEvent.click(screen.getByRole('button', { name: /review this branch/i }))
    expect(openBranchReviewMock).toHaveBeenCalledTimes(1)
  })

  it('the close-view button closes this pane’s view', () => {
    renderHeader(makePane())
    fireEvent.click(screen.getByRole('button', { name: 'Close view' }))
    // A solo pane's view collapsing empties its own chat — the same
    // observable effect TabBar's own close-view control produces.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })

  it('hides the branch-review shortcut and close-view button on the bottom pane', () => {
    renderHeader(makePane({ id: BOTTOM_PANE_ID }))
    expect(screen.queryByRole('button', { name: /review this branch/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Close view' })).not.toBeInTheDocument()
    // The identity header still shows — it isn't a pane-action.
    expect(screen.getByTestId('chat-branch-header')).toBeInTheDocument()
  })
})
