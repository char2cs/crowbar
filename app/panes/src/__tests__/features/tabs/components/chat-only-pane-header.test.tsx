import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import { ChatOnlyPaneHeader } from '@/features/tabs/components/chat-only-pane-header'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup } from '@/features/panes/types/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { Chat, Repo, Workspace } from '@/core/entities/repo-types'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

beforeEach(resetChatStore)

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
  seedRepos([
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
  ])
}

function renderHeader(pane: PaneGroup, wsId: string | null = 'w1') {
  const store = createWorkspaceStore('w1')
  writeChat({ repoId: 'repo-1', ...makeChat() })
  resetWindowPaneStoreForTests()
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

describe('ChatOnlyPaneHeader', () => {
  afterEach(() => {
    seedRepos([])
    vi.clearAllMocks()
  })

  it('carries the window drag region, same as TabBar’s own row', () => {
    renderHeader(makePane())
    const row = screen.getByTestId('pane-top-row')
    expect(row).toHaveAttribute('data-tauri-drag-region')
  })

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

  it('shows the workspace route under the title', () => {
    seedRepos([
      {
        id: 'repo-1',
        name: 'crowbar',
        workspaces: [
          { id: 'w1', branch: 'fix/refactor-chats', parentBranch: 'develop' } as Workspace,
        ],
      } as Repo,
    ])
    renderHeader(makePane())
    expect(screen.getByTestId('chat-branch-header-route')).toHaveTextContent(
      /^crowbar\/develop\/fix\/refactor-chats$/,
    )
  })

  it('offers no branch review, even for a chat that owns its worktree', () => {
    seedRepoChats(true)
    renderHeader(makePane(), 'w1')
    expect(screen.queryByRole('button', { name: /review/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open editor' })).toBeInTheDocument()
  })

  it('renders no close control', () => {
    renderHeader(makePane())
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })

  it('offers Open editor for a thread, which runs on its parent workspace', () => {
    seedRepoChats(false)
    renderHeader(makePane())
    expect(screen.getByRole('button', { name: 'Open editor' })).toBeInTheDocument()
  })

  it('opens its pane’s editor area on Open editor', async () => {
    renderHeader(makePane())
    await userEvent.click(screen.getByRole('button', { name: 'Open editor' }))
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID].editorOpen).toBe(true)
  })

  it('offers no Open editor for a chat with no workspace', () => {
    renderHeader(makePane(), null)
    expect(screen.queryByRole('button', { name: 'Open editor' })).not.toBeInTheDocument()
  })
})
