import { fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { ChatBranchHeader } from '@/features/tabs/components/chat-branch-header'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo, Workspace } from '@/lib/store/sidebar'

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

function makeRepo(workspaces: Workspace[]): Repo {
  return {
    id: 'repo-1',
    projectId: 'proj-1',
    localPath: '',
    defaultBranch: 'main',
    defaultWorkspaceId: 'w1',
    workspaces,
  } as Repo
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

  it("renders the workspace's branch name and change counts on a second line", () => {
    const { container } = renderHeader()
    expect(container.textContent).toContain('feature/pricing-rounding')
    expect(screen.getByText('+34')).toBeInTheDocument()
    expect(screen.getByText('-2')).toBeInTheDocument()
  })

  it('renders only the title when the workspace has no branch to show', () => {
    renderHeader({ wsId: null })
    expect(screen.getByText('Seeding test')).toBeInTheDocument()
    expect(screen.queryByText('feature/pricing-rounding')).not.toBeInTheDocument()
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
