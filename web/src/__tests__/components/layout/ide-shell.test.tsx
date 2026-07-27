// web/src/__tests__/components/layout/IDEShell.test.tsx
import { act, render, screen } from '@testing-library/react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/ide-shell'
import { useSettingsStore } from '@/features/settings/store'

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
  // Surfaces `id` so tests can assert each side gets its own panel identity —
  // react-resizable-panels keys its saved layout by that id.
  ResizablePanel: ({ children, id }: { children: React.ReactNode; id?: string }) => (
    <div data-panel="" data-panel-id={id}>
      {children}
    </div>
  ),
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

  // Regression: moving the sidebar from one side to the other inflated it (321px
  // on the right came back as 640px — SIDEBAR_MAX_PX — on the left). The two
  // orderings were separate ternary branches, so React reconciled the panels
  // POSITIONALLY: slot 0 kept its instance and its auto-generated panel id while
  // swapping roles with slot 2, and react-resizable-panels' layout map — keyed by
  // that id — handed the sidebar the content pane's share of the group.
  describe('moving the sidebar to the other side', () => {
    function setSide(side: 'left' | 'right') {
      act(() => {
        useSettingsStore.setState((state) => {
          state.settings.sidebarPosition = side
        })
      })
    }

    function panelIds(): (string | null)[] {
      return Array.from(document.querySelectorAll('[data-panel]')).map((el) =>
        el.getAttribute('data-panel-id'),
      )
    }

    afterEach(() => setSide('left'))

    it('gives each side its own panel ids, so neither inherits the other side layout', () => {
      setSide('left')
      render(<IDEShell />)
      expect(panelIds()).toEqual(['sidebar-left', 'content-left'])

      setSide('right')
      expect(panelIds()).toEqual(['content-right', 'sidebar-right'])
    })

    it('moves the panels rather than destroying and rebuilding their subtrees', () => {
      setSide('left')
      render(<IDEShell />)
      const sidebarBefore = screen.getByTestId('sidebar-carousel')
      const contentBefore = screen.getByTestId('outlet')

      setSide('right')

      // Same DOM nodes, reordered. A positional reconcile would have unmounted
      // both subtrees — WorkspaceHost, its terminals and Monaco models included —
      // and built fresh ones.
      expect(screen.getByTestId('sidebar-carousel')).toBe(sidebarBefore)
      expect(screen.getByTestId('outlet')).toBe(contentBefore)
    })
  })
})
