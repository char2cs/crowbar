import { nextVersion, resetChatStore, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

const WS = 'w1'

const PROVIDERS: AgentProvider[] = [
  {
    id: 'zeta-cli',
    displayName: 'Zeta',
    icon: '<svg data-p="zeta"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    hasTerminal: true,
    hotswap: false,
    terminalStartHere: false,
  },
]

const chat = (id: string, title: string): AgentChat => ({
  id,
  workspaceId: WS,
  title,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'zeta-cli',
  working: false,
  version: nextVersion(),
  phase: 'dormant',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
})

function seed(chats: AgentChat[]) {
  act(() => {
    useAgentProvidersStore.setState({ status: 'ready', providers: PROVIDERS })
    seedChats(chats)
  })
}

function message(overrides: Partial<ReviewMessage> = {}): ReviewMessage {
  return {
    id: 'msg1',
    author: '',
    isAgent: true,
    body: 'A finding',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function thread(messages: ReviewMessage[]): ReviewThread {
  return {
    id: 'thread1',
    filePath: 'src/foo.ts',
    lineNumber: 5,
    startLine: 5,
    endLine: 5,
    side: 'new',
    messages,
    isResolved: false,
  }
}

const noop = vi.fn().mockResolvedValue(undefined)

function renderThread(messages: ReviewMessage[]) {
  return render(
    <ReviewThreadItem
      thread={thread(messages)}
      wsId={WS}
      onReply={noop}
      onResolve={noop}
      onReopen={noop}
    />,
  )
}

const openChatFn = vi.fn()

beforeEach(() => {
  resetWindowPaneStoreForTests()
  openChatFn.mockReset()
  windowPaneStore.setState((s) => ({
    paneActions: { ...s.paneActions, openChat: openChatFn },
  }))
})

afterEach(() => {
  cleanup()
  resetChatStore()
  resetEntities()
})

describe('review message — which agent wrote it', () => {
  it('names the provider and wears its glyph when the id resolves', () => {
    seed([chat('c1', 'Refactor the parser')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.getByText('Zeta')).toBeInTheDocument()
    expect(screen.queryByText('Agent')).toBeNull()
    const icon = document.querySelector('[data-provider-icon]')
    expect(icon).not.toBeNull()
    expect(icon?.innerHTML).toContain('data-p="zeta"')
  })

  it('falls back to the generic agent rendering when there is no provider id', () => {
    seed([])
    renderThread([message()])

    expect(screen.getByText('Agent')).toBeInTheDocument()
    expect(screen.getByText('agent')).toBeInTheDocument()
    expect(document.querySelector('[data-provider-icon]')).toBeNull()
  })

  it('falls back the same way for a provider this workspace has never heard of', () => {
    seed([])
    renderThread([message({ providerId: 'some-provider-we-never-shipped' })])

    expect(screen.getByText('Agent')).toBeInTheDocument()
    expect(document.querySelector('[data-provider-icon]')).toBeNull()
  })

  it('leaves a human message alone', () => {
    seed([chat('c1', 'Refactor the parser')])
    renderThread([message({ isAgent: false, author: 'alice', providerId: 'zeta-cli' })])

    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.queryByText('Zeta')).toBeNull()
    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
  })
})

describe('review message — the chat it came out of', () => {
  it('shows the chat title and opens that chat when clicked', async () => {
    seed([chat('c1', 'Refactor the parser')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    const link = screen.getByTestId('review-message-chat-link')
    expect(link).toHaveTextContent('Refactor the parser')

    await userEvent.click(link)

    expect(openChatFn).toHaveBeenCalledWith('c1', expect.anything())
  })

  it('calls a chat nobody has named yet what every other surface calls it', () => {
    seed([chat('c1', '')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.getByTestId('review-message-chat-link')).toHaveTextContent(UNTITLED_CHAT_LABEL)
  })

  it('renders inert text — not a link — for a chat that no longer exists', async () => {
    seed([chat('c2', 'A chat that survived')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    const deleted = screen.getByTestId('review-message-chat-deleted')
    expect(deleted).toHaveTextContent('Deleted chat')
    expect(deleted.tagName).not.toBe('BUTTON')
    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
    expect(screen.queryByText(UNTITLED_CHAT_LABEL)).toBeNull()

    await userEvent.click(deleted)
    expect(openChatFn).not.toHaveBeenCalled()
  })

  it('claims nothing while the workspace has no chat list yet', () => {
    act(() => useAgentProvidersStore.setState({ status: 'ready', providers: PROVIDERS }))
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.queryByTestId('review-message-chat-deleted')).toBeNull()
    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
    expect(screen.getByText('Zeta')).toBeInTheDocument()
  })

  it('shows nothing at all when the message names no chat', () => {
    seed([chat('c1', 'Refactor the parser')])
    renderThread([message({ providerId: 'zeta-cli' })])

    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
    expect(screen.queryByTestId('review-message-chat-deleted')).toBeNull()
  })

  it('attributes the root finding and its replies independently', () => {
    seed([chat('c1', 'First chat'), chat('c2', 'Second chat')])
    renderThread([
      message({ id: 'root', providerId: 'zeta-cli', chatId: 'c1' }),
      message({ id: 'reply', providerId: 'zeta-cli', chatId: 'c2' }),
    ])

    const links = screen.getAllByTestId('review-message-chat-link')
    expect(links.map((l) => l.textContent)).toEqual(['First chat', 'Second chat'])
  })
})
