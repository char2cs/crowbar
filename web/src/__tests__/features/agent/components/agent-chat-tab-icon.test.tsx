/**
 * The pane tab's glyph for an agent chat. Before this existed the tab fell through
 * to FileExplorerIcon and every chat wore a generic file icon — the tab told you
 * neither whose agent it was nor that it was mid-turn.
 */
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { AgentChatTabIcon } from '@/features/agent/components/agent-chat-tab-icon'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChat } from '@/features/agent/api/agent-api'

const PROVIDERS = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg data-p="claude"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg data-p="codex"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
]

const chat = (id: string, providerId: string): AgentChat => ({
  id,
  workspaceId: 'w1',
  title: id,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: providerId,
  createdAt: '2026-01-01T00:00:00Z',
})

const state = () => getOrCreateWorkspaceStore('w1').getState()

function seed(chats: AgentChat[] = [chat('c1', 'claude'), chat('c2', 'codex')]) {
  act(() => {
    state().setAgentProviders(PROVIDERS)
    for (const c of chats) state().upsertAgentChat(c)
  })
}

beforeEach(() => seed())
afterEach(() => {
  cleanup()
  destroyWorkspaceStore('w1')
})

describe('AgentChatTabIcon', () => {
  it('wears the chat’s own provider icon', () => {
    const { container, rerender } = render(<AgentChatTabIcon wsId="w1" chatId="c1" />)
    expect(container.querySelector('[data-p="claude"]')).not.toBeNull()

    rerender(<AgentChatTabIcon wsId="w1" chatId="c2" />)
    expect(container.querySelector('[data-p="codex"]')).not.toBeNull()
    expect(container.querySelector('[data-p="claude"]')).toBeNull()
  })

  it('swaps the provider icon for the spinner while the agent is mid-turn, and back', () => {
    const { container } = render(<AgentChatTabIcon wsId="w1" chatId="c1" />)
    expect(screen.queryByRole('status')).toBeNull()

    act(() => state().setAgentChatWorking('c1', true))
    expect(screen.getByRole('status')).toBeTruthy()
    expect(container.querySelector('[data-p="claude"]')).toBeNull()

    act(() => state().setAgentChatWorking('c1', false))
    expect(screen.queryByRole('status')).toBeNull()
    expect(container.querySelector('[data-p="claude"]')).not.toBeNull()
  })

  it('only the working chat’s tab spins', () => {
    const { container } = render(<AgentChatTabIcon wsId="w1" chatId="c2" />)
    act(() => state().setAgentChatWorking('c1', true))
    expect(screen.queryByRole('status')).toBeNull()
    expect(container.querySelector('[data-p="codex"]')).not.toBeNull()
  })

  it('falls back to a chat glyph — never a file icon — when the provider is unknown', () => {
    seed([chat('c3', 'gemini')])
    const { container } = render(<AgentChatTabIcon wsId="w1" chatId="c3" />)
    expect(container.querySelector('[data-provider-icon]')).toBeNull()
    // Specifically the CHAT glyph, not just "some svg" — a file icon is an svg too, and
    // asserting only that one rendered would pass if the fallback regressed to the file
    // icon this feature exists to replace.
    expect(container.querySelector('[data-chat-glyph]')).not.toBeNull()
  })
})
