// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/IDEShell'

vi.mock('@/utils/platform', () => ({
  IS_MAC: true,
  IS_WINDOWS: false,
  IS_LINUX: false,
}))
vi.mock('@/features/workspace/components/WorkspaceView', () => ({
  WorkspaceView: () => <div data-testid="workspace-view" />,
}))
vi.mock('@/components/layout/SidebarTabs', () => ({
  SidebarTabs: () => <div data-testid="sidebar-tabs" />,
}))
vi.mock('@/components/layout/sidebar-project-header', () => ({
  SidebarProjectHeader: () => <div data-testid="sidebar-project-header" />,
}))
vi.mock('@/components/layout/sidebar-nav-icons', () => ({
  SidebarNavIcons: () => <div data-testid="sidebar-nav-icons" />,
}))
vi.mock('@/features/settings/components/settings-dialog', () => ({
  default: () => null,
}))
vi.mock('@/lib/store/sidebar', () => {
  const state = {
    chats: [],
    repos: [],
    collapsedRepos: new Set(),
    addChat: vi.fn(),
    deleteChat: vi.fn(),
    deleteWorkspace: vi.fn(),
    toggleRepo: vi.fn(),
  }
  return {
    useSidebarStore: (selector?: (s: typeof state) => unknown) =>
      selector ? selector(state) : state,
  }
})
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
  Outlet: () => <div data-testid="outlet" />,
}))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarProvider: ({ children }: { children: React.ReactNode }) => <div data-testid="sidebar-provider">{children}</div>,
  Sidebar: ({ children }: { children: React.ReactNode }) => <div data-testid="sidebar">{children}</div>,
  SidebarInset: ({ children }: { children: React.ReactNode }) => <div data-testid="sidebar-inset">{children}</div>,

}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/components/ui/sonner', () => ({
  Toaster: () => null,
}))
vi.mock('@/features/settings/components/font-style-injector', () => ({
  FontStyleInjector: () => null,
}))
vi.mock('@/features/terminal/components/terminal-host', () => ({
  TerminalHost: () => null,
}))

describe('IDEShell', () => {
  it('renders project header', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-project-header')).toBeInTheDocument()
  })

  it('renders SidebarTabs', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-tabs')).toBeInTheDocument()
  })

  it('renders SidebarNavIcons', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-nav-icons')).toBeInTheDocument()
  })

  it('renders Outlet when no workspace or chat is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('renders a resize handle inside the sidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-resize-handle')).toBeInTheDocument()
  })
})
