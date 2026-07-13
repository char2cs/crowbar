// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/ide-shell'

vi.mock('@/utils/platform', () => ({
  IS_MAC: true,
  IS_WINDOWS: false,
  IS_LINUX: false,
}))
vi.mock('@/features/workspace/components/workspace-view', () => ({
  WorkspaceView: () => <div data-testid="workspace-view" />,
}))
vi.mock('@/components/layout/sidebar-tab-bar', () => ({
  SidebarTabBar: () => <div data-testid="sidebar-tab-bar" />,
}))
vi.mock('@/components/layout/sidebar-carousel', () => ({
  SidebarCarousel: () => <div data-testid="sidebar-carousel" />,
}))
vi.mock('@/components/layout/sidebar-project-header', () => ({
  SidebarProjectHeader: () => <div data-testid="sidebar-project-header" />,
}))
vi.mock('@/features/settings/components/settings-dialog', () => ({
  default: () => null,
}))
vi.mock('@/lib/store/sidebar', () => {
  const state = {
    repos: [],
    collapsedRepos: new Set(),
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
  useRouterState: (opts?: { select?: (s: { location: { pathname: string } }) => unknown }) => {
    const state = { location: { pathname: '/' } }
    return opts?.select ? opts.select(state) : state
  },
  Outlet: () => <div data-testid="outlet" />,
}))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="sidebar-provider">{children}</div>
  ),
}))
vi.mock('@/components/ui/resizable', () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizablePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizableHandle: (props: React.HTMLAttributes<HTMLDivElement>) => <div {...props} />,
}))
vi.mock('@/components/error-boundary', () => ({
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

  it('renders SidebarTabBar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-tab-bar')).toBeInTheDocument()
  })

  it('renders SidebarCarousel', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-carousel')).toBeInTheDocument()
  })

  it('renders Outlet when no workspace is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('renders a resize handle inside the sidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-resize-handle')).toBeInTheDocument()
  })
})
