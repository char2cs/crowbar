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
  openBranchReviewForWorkspace: openBranchReviewMock,
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

  // There is no IDE sector at all in this state — this row IS the chat's
  // own toolbar, so it takes the chat's own glass (the progressive-blur
  // dissolve), not TabBar's opaque bg-pane-background.
  it('takes the chat’s own glass, not the IDE sector’s opaque fill', () => {
    renderHeader(makePane())
    const row = screen.getByTestId('pane-top-row')
    expect(row).not.toHaveClass('bg-pane-background')
    expect(screen.getByTestId('edge-dissolve')).toBeInTheDocument()
  })

  it("renders the chat's own identity header", () => {
    renderHeader(makePane())
    expect(screen.getByTestId('chat-branch-header')).toHaveTextContent('My Chat')
  })

  it("the branch-review shortcut opens branch review for THIS pane's own workspace", () => {
    renderHeader(makePane(), 'w1')
    fireEvent.click(screen.getByRole('button', { name: /review this branch/i }))
    expect(openBranchReviewMock).toHaveBeenCalledTimes(1)
    expect(openBranchReviewMock).toHaveBeenCalledWith('w1')
  })

  // Closing a chat/view is a sidebar operation only (Recents' own ×, or a
  // row's own close) — this row offers no close control of its own.
  it('renders no close control', () => {
    renderHeader(makePane())
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })

  it('hides the branch-review shortcut on the bottom pane', () => {
    renderHeader(makePane({ id: BOTTOM_PANE_ID }))
    expect(screen.queryByRole('button', { name: /review this branch/i })).not.toBeInTheDocument()
    // The identity header still shows — it isn't a pane-action.
    expect(screen.getByTestId('chat-branch-header')).toBeInTheDocument()
  })
})
