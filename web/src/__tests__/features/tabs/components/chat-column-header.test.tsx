import { render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { ChatColumnHeader } from '@/features/tabs/components/chat-column-header'
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

function renderHeader() {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    agentChats: { ...s.agentChats, chats: [makeChat()] },
  }))
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(ChatColumnHeader, { chatId: 'chat-1', wsId: 'w1', isBottomPane: false }),
    ),
  )
}

// The chat's own top row when it sits ALONGSIDE a visible editor (side by
// side or stacked presentation) — carries the same window-chrome contract
// TabBar's own row does (it may be the pane's real top-left corner: chat
// renders first), but none of TabBar's pane-level actions — branch-review
// shortcut and close-view already live in the IDE sector's own row.
describe('ChatColumnHeader', () => {
  it('carries the window drag region', () => {
    renderHeader()
    expect(screen.getByTestId('pane-top-row')).toHaveAttribute('data-tauri-drag-region')
  })

  it("renders the chat's own identity header", () => {
    renderHeader()
    expect(screen.getByTestId('chat-branch-header')).toHaveTextContent('My Chat')
  })

  it('takes the translucent chat background, not the opaque IDE-sector one', () => {
    renderHeader()
    const row = screen.getByTestId('pane-top-row')
    expect(row).toHaveClass('bg-chrome-bg')
    expect(row).not.toHaveClass('bg-pane-background')
  })

  it('renders no branch-review shortcut or close-view control of its own', () => {
    renderHeader()
    expect(screen.queryByTestId('branch-review-shortcut')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Close view' })).not.toBeInTheDocument()
  })
})
