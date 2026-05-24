// web/src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { WorkspacesSidebarPanel } from '@/components/layout/WorkspacesSidebarPanel'

const baseProps = {
  userInitials: 'MU',
  chats: [{ id: 'c1', title: 'Test chat', age: '1h' }],
  repos: [],
}

describe('WorkspacesSidebarPanel', () => {
  it('renders chat titles', () => {
    render(<WorkspacesSidebarPanel {...baseProps} />)
    expect(screen.getByText('Test chat')).toBeInTheDocument()
  })

  it('renders New chat button', () => {
    render(<WorkspacesSidebarPanel {...baseProps} />)
    expect(screen.getByText('New chat')).toBeInTheDocument()
  })

  it('renders repo names', () => {
    const repos = [{ id: 'r1', name: 'payment-api', avatarLabel: 'P', avatarColor: '#6366f1', workspaces: [] }]
    render(<WorkspacesSidebarPanel {...baseProps} repos={repos} />)
    expect(screen.getByText('payment-api')).toBeInTheDocument()
  })

  it('calls onChatClick when a chat row is clicked', () => {
    const onChatClick = vi.fn()
    render(<WorkspacesSidebarPanel {...baseProps} onChatClick={onChatClick} />)
    screen.getByText('Test chat').click()
    expect(onChatClick).toHaveBeenCalledWith('c1')
  })
})
