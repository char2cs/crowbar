import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ChatTree } from '@/components/layout/chat-tree'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import type { ProjectChat } from '@/lib/store/sidebar'
import { apiFetch } from '@/lib/api'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn().mockResolvedValue([]),
}))
vi.mock('@tanstack/react-router', () => ({
  useRouterState: vi.fn().mockReturnValue('/'),
  useNavigate: vi.fn().mockReturnValue(vi.fn()),
}))
vi.mock('@/components/layout/chat-tree-item', () => ({
  ChatTreeItem: ({
    node,
  }: {
    node: { chat: ProjectChat; children: { chat: ProjectChat; children: unknown[] }[] }
  }) => (
    <div>
      <div data-testid={`chat-${node.chat.id}`}>{node.chat.title}</div>
      {node.children.map((child) => (
        <div key={child.chat.id} data-testid={`chat-${child.chat.id}`}>
          {child.chat.title}
        </div>
      ))}
    </div>
  ),
}))
vi.mock('@/components/layout/chat-tree-context', () => ({
  ChatTreeProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  useChatTreeContext: () => ({
    draggingChat: null,
    dragPos: null,
    hoverTrash: false,
  }),
  performCreateChat: vi.fn(),
}))

const CHATS: ProjectChat[] = [
  { id: 'c1', wsId: 'ws1', title: 'First chat', age: '5m', status: 'idle', type: 'chat' },
  {
    id: 'c2',
    wsId: 'ws1',
    title: 'Forked chat',
    age: '2h',
    parentId: 'c1',
    status: 'idle',
    type: 'chat',
  },
  { id: 'c3', wsId: 'ws1', title: 'Root chat 2', age: '1d', status: 'agent-running', type: 'chat' },
]

describe('ChatTree', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
  })

  it('renders chats for the active workspace from store', () => {
    useSidebarStore.setState({ chats: CHATS })
    render(<ChatTree wsId="ws1" />)
    expect(screen.getByTestId('chat-c1')).toBeInTheDocument()
    expect(screen.getByTestId('chat-c2')).toBeInTheDocument()
    expect(screen.getByTestId('chat-c3')).toBeInTheDocument()
  })

  it('renders only chats matching wsId', () => {
    useSidebarStore.setState({
      chats: [
        ...CHATS,
        {
          id: 'c4',
          wsId: 'other-ws',
          title: 'Other workspace chat',
          age: '3d',
          status: 'idle',
          type: 'chat',
        },
      ],
    })
    render(<ChatTree wsId="ws1" />)
    expect(screen.queryByText('Other workspace chat')).not.toBeInTheDocument()
  })

  it('renders a New chat button', () => {
    render(<ChatTree wsId="ws1" />)
    expect(screen.getByRole('button', { name: /new chat/i })).toBeInTheDocument()
  })

  it('fetches the chat list for the workspace', async () => {
    vi.mocked(apiFetch).mockClear()
    render(<ChatTree wsId="ws1" />)
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith('/v0/workspaces/ws1/chats')
    })
  })

  it('does not fetch when wsId is empty (no /v0/workspaces//chats)', async () => {
    vi.mocked(apiFetch).mockClear()
    render(<ChatTree wsId="" />)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(apiFetch).not.toHaveBeenCalled()
  })
})
