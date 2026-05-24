// web/src/__tests__/components/layout/SidebarTabs.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { SidebarTabs } from '@/components/layout/SidebarTabs'

vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="file-explorer" />,
}))
vi.mock('@/features/git/components/git-view', () => ({
  default: () => <div data-testid="git-view" />,
}))
vi.mock('@/components/layout/WorkspacesSidebarPanel', () => ({
  WorkspacesSidebarPanel: () => <div data-testid="workspaces-panel" />,
}))

const baseProps = {
  userInitials: 'MU',
  chats: [],
  repos: [],
  activeWorkspaceRepoPath: '/repo',
}

describe('SidebarTabs', () => {
  it('renders Workspaces tab by default', () => {
    render(<SidebarTabs {...baseProps} />)
    expect(screen.getByTestId('workspaces-panel')).toBeInTheDocument()
  })

  it('renders Files tab when clicked', () => {
    render(<SidebarTabs {...baseProps} />)
    fireEvent.click(screen.getByRole('tab', { name: /files/i }))
    expect(screen.getByTestId('file-explorer')).toBeInTheDocument()
  })

  it('renders Git tab when clicked', () => {
    render(<SidebarTabs {...baseProps} />)
    fireEvent.click(screen.getByRole('tab', { name: /git/i }))
    expect(screen.getByTestId('git-view')).toBeInTheDocument()
  })

  it('shows three tab triggers', () => {
    render(<SidebarTabs {...baseProps} />)
    expect(screen.getAllByRole('tab').length).toBe(3)
  })
})
