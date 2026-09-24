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
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Chat, Repo } from '@/lib/store/sidebar'

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
    viewId: null,
    ...overrides,
  }
}

/** Seeds the sidebar tree so `chat-1` is either a THREAD hanging off `w1`'s
 *  own owning chat, or the chat that owns `w1` itself. */
function seedRepoChats(chatOwnsWorktree: boolean) {
  const owner: Chat = {
    id: chatOwnsWorktree ? 'chat-1' : 'chat-main',
    repoId: 'repo-1',
    title: 'main',
    order: 0,
    workspaceId: 'w1',
    ownsWorktree: true,
  }
  const chats: Chat[] = chatOwnsWorktree
    ? [owner]
    : [
        owner,
        {
          id: 'chat-1',
          repoId: 'repo-1',
          title: 'My Chat',
          order: 1,
          workspaceId: 'w1',
          ownsWorktree: false,
          parentId: 'chat-main',
        },
      ]
  useSidebarStore.setState({
    repos: [
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: '#000',
        workspaces: [],
        defaultWorkspaceId: 'w1',
        defaultOwningChatId: owner.id,
        chats,
      } as Repo,
    ],
  })
}

function renderHeader(pane: PaneGroup, wsId: string | null = 'w1') {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [makeChat()] },
  }))
  resetWindowPaneStoreForTests()
  // The bottom tray never holds a chat; the header is handed its pane directly.
  if (pane.id !== BOTTOM_PANE_ID && pane.chatId) {
    windowPaneStore.getState().paneActions.openChat(pane.chatId)
  }
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
    useSidebarStore.setState({ repos: [] })
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

  it("the branch-review shortcut opens branch review for THIS pane's own workspace and pane", () => {
    renderHeader(makePane(), 'w1')
    fireEvent.click(screen.getByRole('button', { name: /review this branch/i }))
    expect(openBranchReviewMock).toHaveBeenCalledTimes(1)
    // The pane id too, not just the workspace: openContent (buffer-slice.ts)
    // always adds the new tab to whichever pane is currently ACTIVE, so a
    // click on an INACTIVE pane's own shortcut must assert this pane active
    // first — passing only the workspace correctly tagged the buffer but
    // still let it land in the wrong pane's tab strip (live-reported).
    expect(openBranchReviewMock).toHaveBeenCalledWith('w1', ROOT_PANE_ID)
  })

  // Closing a chat/view is a sidebar operation only (Recents' own ×, or a
  // row's own close) — this row offers no close control of its own.
  it('renders no close control', () => {
    renderHeader(makePane())
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })

  // Live-reported: a THREAD's own header offered "Review this branch". A
  // thread owns no branch — it runs on the one its parent owns — so the
  // affordance belongs to the owning chat's header only, exactly as the tree
  // only ever draws Fork/branch chrome on a `kind: 'branch'` row.
  it('hides the branch-review shortcut for a thread that owns no worktree', () => {
    seedRepoChats(false)
    renderHeader(makePane())
    expect(screen.queryByRole('button', { name: /review this branch/i })).not.toBeInTheDocument()
    // The identity header still shows — only the branch action is gone.
    expect(screen.getByTestId('chat-branch-header')).toBeInTheDocument()
  })

  it('still shows the branch-review shortcut for a chat that owns its worktree', () => {
    seedRepoChats(true)
    renderHeader(makePane())
    expect(screen.getByRole('button', { name: /review this branch/i })).toBeInTheDocument()
  })

  it('hides the branch-review shortcut on the bottom pane', () => {
    renderHeader(makePane({ id: BOTTOM_PANE_ID }))
    expect(screen.queryByRole('button', { name: /review this branch/i })).not.toBeInTheDocument()
    // The identity header still shows — it isn't a pane-action.
    expect(screen.getByTestId('chat-branch-header')).toBeInTheDocument()
  })
})
