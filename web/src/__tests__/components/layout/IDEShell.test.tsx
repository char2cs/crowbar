// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/IDEShell'

// Mock heavy features — just verify composition
vi.mock('@/features/workspace/components/WorkspaceView', () => ({
  WorkspaceView: () => <div data-testid="workspace-view" />,
}))
vi.mock('@/components/layout/SidebarTabs', () => ({
  SidebarTabs: () => <div data-testid="sidebar-tabs" />,
}))
vi.mock('@/components/layout/SidebarHeader', () => ({
  SidebarHeader: () => <div data-testid="sidebar-header" />,
}))
vi.mock('@/features/settings/components/settings-dialog', () => ({
  default: () => null,
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
  Outlet: () => <div data-testid="outlet" />,
}))
vi.mock('@/components/ui/resizable', () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizablePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => null,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

describe('IDEShell', () => {
  it('renders sidebar header', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-header')).toBeInTheDocument()
  })

  it('renders SidebarTabs', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-tabs')).toBeInTheDocument()
  })

  it('renders Outlet when no workspace or chat is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })
})
