import { fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { ChatHead } from '@/features/tabs/components/chat-head'
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

function renderChatHead(props: Partial<ComponentProps<typeof ChatHead>> = {}) {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [makeChat({ id: 'chat-1' })] },
  }))
  const onSelect = props.onSelect ?? (() => {})
  const view = render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatHead, {
        chatId: 'chat-1',
        isActive: false,
        onSelect,
        ...props,
      }),
    ),
  )
  return { store, onSelect, ...view }
}

// This is the pane's chat-identity header (spec §7.1). It renders through the
// SAME `Tab` primitive (`@/components/ui/tabs`, `variant="underline"`) that
// tab-bar-item.tsx uses for file tabs, so the two surfaces can never drift
// into two different "active" treatments — one shared implementation, two
// call sites (tab-bar-item.test.tsx covers the other).
describe('ChatHead underline restyle', () => {
  it('active chat head is flat, not a filled rounded pill', () => {
    renderChatHead({ isActive: true })
    const head = screen.getByTestId('chat-head')
    expect(head).not.toHaveClass('rounded-full')
    expect(head).not.toHaveClass('bg-background')
    expect(head).not.toHaveClass('shadow-sm')
  })

  it('inactive chat head is flat with muted text, no fill', () => {
    renderChatHead({ isActive: false })
    const head = screen.getByTestId('chat-head')
    expect(head).not.toHaveClass('rounded-full')
    expect(head).not.toHaveClass('bg-background')
    expect(head).toHaveClass('text-muted-foreground')
  })

  it('active chat head carries the same 2px primary underline bar as an active file tab', () => {
    renderChatHead({ isActive: true })
    const bar = screen.getByTestId('tab-underline')
    expect(bar).toHaveClass('bg-primary')
    expect(bar).toHaveClass('h-0.5')
  })

  it('inactive chat head has no underline bar', () => {
    renderChatHead({ isActive: false })
    expect(screen.queryByTestId('tab-underline')).not.toBeInTheDocument()
  })

  it('renders the chat title and fires onSelect when clicked', () => {
    const onSelect = vi.fn()
    renderChatHead({ onSelect })
    expect(screen.getByText('My Chat')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('chat-head'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  // An untitled chat's `title` field is an empty string, not null/undefined —
  // `??` doesn't catch it. Same bug class Task 12 fixed in recents-band.tsx;
  // this is chat-head.tsx's own instance of it (spec §7.1: the pane's chat
  // head must never render a blank label).
  it('falls back to UNTITLED_CHAT_LABEL when the chat title is an empty string', () => {
    const store = createWorkspaceStore('w1')
    store.setState((s) => ({
      ...s,
      agentChats: { ...s.agentChats, chats: [makeChat({ id: 'chat-1', title: '' })] },
    }))
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(ChatHead, { chatId: 'chat-1', isActive: false, onSelect: () => {} }),
      ),
    )
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })
})
