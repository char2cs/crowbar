// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/IDEShell'

// Mock heavy features — just verify composition
vi.mock('@/features/layout/components/sidebar/main-sidebar', () => ({
  MainSidebar: ({ children }: { children: React.ReactNode }) => (
    <aside data-testid="main-sidebar">{children}</aside>
  ),
}))
vi.mock('@/features/panes/components/split-view-root', () => ({
  SplitViewRoot: () => <div data-testid="split-view-root" />,
}))
vi.mock('@/components/layout/SidebarTabs', () => ({
  SidebarTabs: () => <div data-testid="sidebar-tabs" />,
}))
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: () => ({
    chats: [],
    repos: [],
    collapsedRepos: new Set(),
    addChat: vi.fn(),
    deleteChat: vi.fn(),
    deleteWorkspace: vi.fn(),
    toggleRepo: vi.fn(),
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
}))

describe('IDEShell', () => {
  it('renders MainSidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('main-sidebar')).toBeInTheDocument()
  })

  it('renders SplitViewRoot', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('split-view-root')).toBeInTheDocument()
  })

  it('renders SidebarTabs inside MainSidebar', () => {
    render(<IDEShell />)
    const sidebar = screen.getByTestId('main-sidebar')
    expect(sidebar.querySelector('[data-testid="sidebar-tabs"]')).toBeTruthy()
  })
})
