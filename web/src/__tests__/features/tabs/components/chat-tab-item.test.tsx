import { fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { ChatTabItem } from '@/features/tabs/components/chat-tab-item'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

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

function renderChatTab(overrides: { isActive?: boolean; onSelect?: () => void } = {}) {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [makeChat()] },
  }))
  const onSelect = overrides.onSelect ?? (() => {})
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatTabItem, {
        chatId: 'chat-1',
        isActive: overrides.isActive ?? false,
        onSelect,
      }),
    ),
  )
}

// The synthetic "Chat" entry in the IDE sector's tab strip (collapsed/'tabs'
// presentation only) — per the redesign, the chat is "just another tab" you
// can switch away from. Ghost-styled like its real-tab neighbours, but never
// closable or draggable — see tab-bar.tsx's own render site for that.
describe('ChatTabItem', () => {
  it('renders the chat title', () => {
    renderChatTab()
    expect(screen.getByText('My Chat')).toBeInTheDocument()
  })

  it('falls back to UNTITLED_CHAT_LABEL for an empty title', () => {
    const store = createWorkspaceStore('w1')
    store.setState((s) => ({
      ...s,
      agentChats: { ...s.agentChats, chats: [makeChat({ title: '' })] },
    }))
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(ChatTabItem, { chatId: 'chat-1', isActive: false, onSelect: () => {} }),
      ),
    )
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })

  it('fires onSelect when clicked', () => {
    const onSelect = vi.fn()
    renderChatTab({ onSelect })
    fireEvent.click(screen.getByTestId('chat-tab-item'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('renders no close affordance — it is never closable', () => {
    renderChatTab()
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })

  it('uses the ghost tab styling — a persistent fill when active, none when not', () => {
    const { rerender } = renderChatTab({ isActive: true })
    const tabActive = screen.getByTestId('chat-tab-item')
    expect(tabActive).toHaveClass('bg-sidebar-element-hover')

    const store = createWorkspaceStore('w1')
    store.setState((s) => ({
      ...s,
      agentChats: { ...s.agentChats, chats: [makeChat()] },
    }))
    rerender(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(ChatTabItem, { chatId: 'chat-1', isActive: false, onSelect: () => {} }),
      ),
    )
    expect(screen.getByTestId('chat-tab-item')).not.toHaveClass('bg-sidebar-element-hover')
  })
})
