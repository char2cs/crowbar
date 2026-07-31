/**
 * Agent attribution on a review message: WHICH agent wrote it, and WHICH chat it
 * came out of.
 *
 * Both ids are absent on every human message and on every agent message written
 * before attribution existed — which is most of them, forever — so the fallback
 * is not an edge case here, it is the common path, and it must be byte-for-byte
 * what this component rendered before: a generic "Agent".
 *
 * Nothing in the component may branch on a provider id's VALUE. These tests use
 * ids the code has never heard of on purpose: a provider is only ever a row in
 * the workspace's provider list, resolved by id, whatever that id turns out to be.
 */
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

const { openAgentChatFn } = vi.hoisted(() => ({ openAgentChatFn: vi.fn() }))
vi.mock('@/features/agent/lib/open-agent-chat', () => ({
  openAgentChat: (...a: unknown[]) => openAgentChatFn(...a),
}))

import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

const WS = 'w1'

// Deliberately not 'claude'/'codex': if any of this passed only for the two
// providers Crowbar ships with today, that would be the bug.
const PROVIDERS: AgentProvider[] = [
  {
    id: 'zeta-cli',
    displayName: 'Zeta',
    icon: '<svg data-p="zeta"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
]

const chat = (id: string, title: string): AgentChat => ({
  id,
  workspaceId: WS,
  title,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'zeta-cli',
  createdAt: '2026-01-01T00:00:00Z',
})

const state = () => getOrCreateWorkspaceStore(WS).getState()

function seed(chats: AgentChat[]) {
  act(() => {
    state().setAgentProviders(PROVIDERS)
    for (const c of chats) state().upsertAgentChat(c)
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

beforeEach(() => {
  openAgentChatFn.mockReset()
})

afterEach(() => {
  cleanup()
  destroyWorkspaceStore(WS)
})

describe('review message — which agent wrote it', () => {
  it('names the provider and wears its glyph when the id resolves', () => {
    seed([chat('c1', 'Refactor the parser')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.getByText('Zeta')).toBeInTheDocument()
    expect(screen.queryByText('Agent')).toBeNull()
    // ProviderIcon inlines the backend's own SVG markup.
    const icon = document.querySelector('[data-provider-icon]')
    expect(icon).not.toBeNull()
    expect(icon?.innerHTML).toContain('data-p="zeta"')
  })

  // THE COMMON PATH, FOREVER. Every agent message written before attribution
  // existed carries no provider id, and must render exactly as it always did.
  it('falls back to the generic agent rendering when there is no provider id', () => {
    seed([])
    renderThread([message()])

    expect(screen.getByText('Agent')).toBeInTheDocument()
    expect(screen.getByText('agent')).toBeInTheDocument() // the badge is untouched
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

    expect(openAgentChatFn).toHaveBeenCalledWith(getOrCreateWorkspaceStore(WS), WS, 'c1')
  })

  it('calls a chat nobody has named yet what every other surface calls it', () => {
    seed([chat('c1', '')])
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.getByTestId('review-message-chat-link')).toHaveTextContent(UNTITLED_CHAT_LABEL)
  })

  // A chat id lives on the message forever; the chat does not. An id pointing at
  // a deleted chat must not render as something that opens it — and "deleted" is
  // a different state from "untitled", so it must not borrow that word either.
  it('renders inert text — not a link — for a chat that no longer exists', async () => {
    seed([chat('c2', 'A chat that survived')]) // c1 was deleted; the message still names it
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    const deleted = screen.getByTestId('review-message-chat-deleted')
    expect(deleted).toHaveTextContent('Deleted chat')
    expect(deleted.tagName).not.toBe('BUTTON')
    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
    expect(screen.queryByText(UNTITLED_CHAT_LABEL)).toBeNull()

    await userEvent.click(deleted)
    expect(openAgentChatFn).not.toHaveBeenCalled()
  })

  // "Deleted" is a claim, and an unseeded store is not evidence for it. The chat
  // list arrives asynchronously with the workspace, so a review opened on a cold
  // store would otherwise announce every chat as deleted and then take it back.
  it('claims nothing while the workspace has no chat list yet', () => {
    seed([]) // providers seeded, chats not yet
    renderThread([message({ providerId: 'zeta-cli', chatId: 'c1' })])

    expect(screen.queryByTestId('review-message-chat-deleted')).toBeNull()
    expect(screen.queryByTestId('review-message-chat-link')).toBeNull()
    // The provider half of the attribution does not wait on chats.
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
