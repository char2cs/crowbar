import { fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { CHAT_TITLE_PENDING_LABEL } from '@/features/agent/hooks/use-chat-title'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { ChatBranchHeader } from '@/features/tabs/components/chat-branch-header'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'

vi.mock('@/components/sidebar/lib/row-actions', () => ({
  performRenameChat: vi.fn().mockResolvedValue(undefined),
}))

function makeChat(overrides: Partial<AgentChat> = {}): AgentChat {
  return {
    id: 'chat-1',
    workspaceId: 'w1',
    title: 'Seeding test',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    createdAt: new Date().toISOString(),
    parentId: '',
    ...overrides,
  } as AgentChat
}

function makeWorkspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: 'w1',
    branch: 'feature/pricing-rounding',
    added: 34,
    deleted: 2,
    localPath: '',
    status: 'new',
    ...overrides,
  } as Workspace
}

function makeRepo(workspaces: Workspace[], chats?: Chat[]): Repo {
  return {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'crowbar',
    localPath: '',
    defaultBranch: 'main',
    defaultWorkspaceId: 'w1',
    workspaces,
    chats,
  } as Repo
}

function makeSidebarChat(overrides: Partial<Chat> = {}): Chat {
  return { id: 'chat-1', repoId: 'repo-1', title: 'Seeding test', order: 0, ...overrides }
}

function renderHeader({
  working = false,
  title = 'Seeding test',
  wsId = 'w1' as string | null,
  repos = [makeRepo([makeWorkspace()])],
}: {
  working?: boolean
  title?: string
  wsId?: string | null
  repos?: Repo[]
} = {}) {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: {
      ...s.agentChats,
      chats: [makeChat({ title })],
      working: { 'chat-1': working },
    },
  }))
  useSidebarStore.setState({ repos })

  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatBranchHeader, { chatId: 'chat-1', wsId }),
    ),
  )
}

/** A store whose chat list simply does not carry `chat-1` — a pane restored
 *  from a saved layout, or a list that raced it. */
function renderHeaderWithoutChatRecord() {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [], listSeeded: true },
  }))
  useSidebarStore.setState({ repos: [makeRepo([makeWorkspace()])] })

  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatBranchHeader, { chatId: 'chat-1', wsId: 'w1' }),
    ),
  )
}

afterEach(() => {
  useSidebarStore.setState({ repos: [] })
  vi.clearAllMocks()
})

// The chat interface's own identity header (replaces ChatHead in this
// position — spec: "Branch icon, branch name... same style that rows use,
// but without the row chrome"). Reuses the sidebar row's rule-6 two-line
// shape (title, then branch name + change counts) but never draws the row's
// hover/selection chrome — it isn't a selectable row or tab here.
describe('ChatBranchHeader', () => {
  it('renders the chat title', () => {
    renderHeader()
    expect(screen.getByText('Seeding test')).toBeInTheDocument()
  })

  it('falls back to UNTITLED_CHAT_LABEL for an empty title', () => {
    renderHeader({ title: '' })
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })

  // Live-reported: the pane header read "Untitled chat" for a chat the sidebar
  // and the daemon both called "repochat-RN", and only a reload fixed it — the
  // header's `?.title || UNTITLED_CHAT_LABEL` made "this store has not got the
  // record" indistinguishable from "this chat has no name". A gap must look
  // like a gap.
  it('shows the pending mark, not UNTITLED_CHAT_LABEL, for a chat the store has no record of', () => {
    renderHeaderWithoutChatRecord()
    const title = screen.getByTestId('chat-branch-header-title')
    expect(title).toHaveAttribute('data-title-pending', 'true')
    expect(title).toHaveTextContent(CHAT_TITLE_PENDING_LABEL)
    expect(screen.queryByText(UNTITLED_CHAT_LABEL)).not.toBeInTheDocument()
  })

  it('offers no rename for a chat the store has no record of — there is no title to edit', () => {
    renderHeaderWithoutChatRecord()
    fireEvent.doubleClick(screen.getByTestId('chat-branch-header'))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it("renders the workspace's branch name and change counts on a second line", () => {
    const { container } = renderHeader()
    expect(container.textContent).toContain('feature/pricing-rounding')
    expect(screen.getByText('+34')).toBeInTheDocument()
    expect(screen.getByText('-2')).toBeInTheDocument()
  })

  it('shows the counts after the route with no dash between them', () => {
    renderHeader()
    const line = screen.getByTestId('chat-branch-header-route').parentElement!
    expect(line.textContent).toBe('crowbar/feature/pricing-rounding +34 -2')
  })

  it('routes the workspace as repo / parent / branch on one line, branch last', () => {
    renderHeader({
      repos: [makeRepo([makeWorkspace({ parentId: 'w0', parentBranch: 'develop' })])],
    })
    const route = screen.getByTestId('chat-branch-header-route')
    expect(route).toHaveTextContent(/^crowbar\/develop\/feature\/pricing-rounding$/)
    expect(route.lastElementChild).toHaveTextContent(/^feature\/pricing-rounding$/)
    expect(route).not.toContainElement(screen.getByTestId('chat-branch-header-title'))
  })

  it('shows the repo icon in the left column, outside the title and route lines', () => {
    renderHeader()
    const repoIcon = screen.getByTestId('chat-branch-header-repo-icon')
    const title = screen.getByTestId('chat-branch-header-title')
    const route = screen.getByTestId('chat-branch-header-route')
    expect(repoIcon.parentElement).toBe(screen.getByTestId('chat-branch-header'))
    expect(repoIcon.nextElementSibling).toContainElement(title)
    expect(repoIcon).not.toContainElement(route)
    expect(route).not.toContainElement(repoIcon)
  })

  it('leads the route line with the chat glyph and leaves the title line bare', () => {
    renderHeader()
    const glyph = screen.getByTestId('chat-branch-header-glyph')
    const route = screen.getByTestId('chat-branch-header-route')
    expect(glyph.nextElementSibling).toBe(route)
    expect(glyph.parentElement).not.toContainElement(screen.getByTestId('chat-branch-header-title'))
    expect(screen.getByTestId('chat-branch-header-title').querySelector('svg')).toBeNull()
    expect(route.querySelector('svg')).toBeNull()
    expect(route).toHaveTextContent(/^crowbar\/feature\/pricing-rounding$/)
  })

  it('omits the parent segment for a branch with no parent', () => {
    renderHeader()
    expect(screen.getByTestId('chat-branch-header-route')).toHaveTextContent(
      /^crowbar\/feature\/pricing-rounding$/,
    )
  })

  it('omits a parent segment that names the branch itself', () => {
    renderHeader({
      repos: [makeRepo([makeWorkspace({ parentBranch: 'feature/pricing-rounding' })])],
    })
    expect(screen.getByTestId('chat-branch-header-route')).toHaveTextContent(
      /^crowbar\/feature\/pricing-rounding$/,
    )
  })

  it('renders only the title when the workspace has no branch to show', () => {
    renderHeader({ wsId: null })
    expect(screen.getByText('Seeding test')).toBeInTheDocument()
    expect(screen.queryByText('feature/pricing-rounding')).not.toBeInTheDocument()
    expect(screen.queryByTestId('chat-branch-header-route')).not.toBeInTheDocument()
  })

  it('shows a working spinner in place of the branch glyph while the chat is working', () => {
    renderHeader({ working: true })
    expect(screen.getByTestId('chat-branch-header-glyph').querySelector('svg')).toBeTruthy()
    expect(screen.queryByTestId('chat-branch-header-branch-icon')).not.toBeInTheDocument()
  })

  it('shows the plain branch glyph when the chat is not working', () => {
    renderHeader({ working: false })
    expect(screen.getByTestId('chat-branch-header-branch-icon')).toBeInTheDocument()
  })

  // Live-reported: a THREAD ("Hi claude", nested under main's own chat and
  // running on main's ground) drew the branch mark, because the header keyed
  // off the workspace the chat RUNS IN — which a thread inherits from its
  // parent — instead of the workspace it OWNS. Same rule `RowGlyph`
  // (sidebar-row.tsx) already applies in the tree: no `ownsWorktree`, no
  // branch mark.
  it('draws the chat bubble, not the branch mark, for a thread that owns no worktree', () => {
    renderHeader({
      repos: [
        makeRepo(
          [makeWorkspace()],
          [
            makeSidebarChat({ id: 'chat-main', workspaceId: 'w1', ownsWorktree: true }),
            makeSidebarChat({
              id: 'chat-1',
              workspaceId: 'w1',
              ownsWorktree: false,
              parentId: 'chat-main',
            }),
          ],
        ),
      ],
    })
    expect(screen.queryByTestId('chat-branch-header-branch-icon')).not.toBeInTheDocument()
    expect(screen.getByTestId('chat-branch-header-chat-icon')).toBeInTheDocument()
  })

  it('still draws the branch mark for a chat that owns its own worktree', () => {
    renderHeader({
      repos: [
        makeRepo(
          [makeWorkspace({ owningChatId: 'chat-1' })],
          [makeSidebarChat({ id: 'chat-1', workspaceId: 'w1', ownsWorktree: true })],
        ),
      ],
    })
    expect(screen.getByTestId('chat-branch-header-branch-icon')).toBeInTheDocument()
    expect(screen.queryByTestId('chat-branch-header-chat-icon')).not.toBeInTheDocument()
  })

  it('carries none of the sidebar row hover/selection chrome', () => {
    renderHeader()
    const header = screen.getByTestId('chat-branch-header')
    expect(header.className).not.toMatch(/hover:bg-sidebar-element-hover/)
  })

  it('has no fixed height/padding of its own — the caller controls sizing via className', () => {
    renderHeader()
    const header = screen.getByTestId('chat-branch-header')
    expect(header.className).not.toMatch(/\bh-8\b/)
    expect(header.className).not.toMatch(/\bpx-2\.5\b/)
  })

  it('merges a caller-supplied className onto its own root element', () => {
    const store = createWorkspaceStore('w1')
    store.setState((s) => ({
      ...s,
      agentChats: { ...s.agentChats, chats: [makeChat()] },
    }))
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(ChatBranchHeader, {
          chatId: 'chat-1',
          wsId: null,
          className: 'h-full min-w-0 flex-1',
        }),
      ),
    )
    const header = screen.getByTestId('chat-branch-header')
    expect(header).toHaveClass('h-full', 'min-w-0', 'flex-1')
  })

  it('double-click enters rename mode with the current title pre-filled', () => {
    renderHeader()
    fireEvent.doubleClick(screen.getByTestId('chat-branch-header'))
    expect(screen.getByDisplayValue('Seeding test')).toBeInTheDocument()
  })

  describe('a chat created directly under a project', () => {
    function seedHomeChat(
      projects: unknown[] = [{ id: 'proj-1', name: 'Crowbar Seed', path: '/p' }],
    ) {
      useHomeTreeStore.setState({
        trees: { 'proj-1': { chats: [makeSidebarChat()], folders: [] } },
      })
      useProjectDataStore.setState({ data: success(projects as never) })
    }
    afterEach(() => {
      useHomeTreeStore.setState({ trees: {} })
      useProjectDataStore.setState({ data: { status: 'idle' } as never })
    })

    it('shows the project icon on the left and the project name as the path', () => {
      seedHomeChat()
      renderHeader({ wsId: null, repos: [] })
      const header = screen.getByTestId('chat-branch-header')
      const icon = screen.getByTestId('chat-branch-header-project-icon')
      expect(icon.parentElement).toBe(header)
      const route = screen.getByTestId('chat-branch-header-route')
      expect(route).toHaveTextContent(/^Crowbar Seed$/)
      expect(route.previousElementSibling).toBe(screen.getByTestId('chat-branch-header-glyph'))
    })

    it('keeps the plain fallback until the project resolves', () => {
      seedHomeChat([])
      renderHeader({ wsId: null, repos: [] })
      expect(screen.queryByTestId('chat-branch-header-project-icon')).not.toBeInTheDocument()
      expect(screen.queryByTestId('chat-branch-header-route')).not.toBeInTheDocument()
    })
  })

  it('keeps the route line and repo icon in place while the title is edited', () => {
    renderHeader()
    fireEvent.doubleClick(screen.getByTestId('chat-branch-header'))
    const input = screen.getByDisplayValue('Seeding test')
    expect(screen.getByTestId('chat-branch-header-route')).toBeInTheDocument()
    expect(screen.getByTestId('chat-branch-header-repo-icon')).toBeInTheDocument()
    expect(input.parentElement).toContainElement(screen.getByTestId('chat-branch-header-route'))
  })

  it('confirming a changed name renames the chat', async () => {
    const { performRenameChat } = await import('@/components/sidebar/lib/row-actions')
    renderHeader()
    fireEvent.doubleClick(screen.getByTestId('chat-branch-header'))
    const input = screen.getByDisplayValue('Seeding test')
    fireEvent.change(input, { target: { value: 'Renamed chat' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(performRenameChat).toHaveBeenCalledWith('chat-1', 'Renamed chat')
    expect(screen.queryByDisplayValue('Renamed chat')).not.toBeInTheDocument()
  })

  it('cancelling a rename leaves the title untouched', async () => {
    const { performRenameChat } = await import('@/components/sidebar/lib/row-actions')
    renderHeader()
    fireEvent.doubleClick(screen.getByTestId('chat-branch-header'))
    fireEvent.keyDown(screen.getByDisplayValue('Seeding test'), { key: 'Escape' })
    expect(performRenameChat).not.toHaveBeenCalled()
    expect(screen.getByText('Seeding test')).toBeInTheDocument()
  })
})
