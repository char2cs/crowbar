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
vi.mock('@/components/layout/sidebar-carousel', () => ({
  SidebarCarousel: () => <div data-testid="sidebar-carousel" />,
}))
const { sidebarProjectHeaderMock } = vi.hoisted(() => ({
  sidebarProjectHeaderMock: vi.fn(),
}))
vi.mock('@/components/layout/sidebar-project-header', () => ({
  SidebarProjectHeader: (props: unknown) => {
    sidebarProjectHeaderMock(props)
    return <div data-testid="sidebar-project-header" />
  },
}))
vi.mock('@/components/layout/sidebar-tree-surface', () => ({
  SidebarTreeSurface: () => <div data-testid="sidebar-tree-surface" />,
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

  it('renders SidebarCarousel', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-carousel')).toBeInTheDocument()
  })

  it("renders SidebarTreeSurface (SpaceScroller's real mount point) between the header and the carousel", () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-tree-surface')).toBeInTheDocument()
  })

  it('renders Outlet when no workspace is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('renders a resize handle inside the sidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-resize-handle')).toBeInTheDocument()
  })

  // task-10's placement override (project marks as the sidebar's own true
  // last element, below the floating file-explorer card) is reverted: spec
  // §2 rules that card "ALWAYS THE LAST ELEMENT... Nothing goes below it".
  // The marks render inside SidebarProjectHeader's own window-chrome row
  // now (its own component test covers the marks themselves) — IDEShell's
  // job is just wiring the project data through to it, and no longer
  // mounting a separate footer sibling at all.
  it('passes project data through to SidebarProjectHeader instead of mounting a separate footer', () => {
    render(<IDEShell />)
    expect(screen.queryByTestId('sidebar-footer')).not.toBeInTheDocument()
    expect(sidebarProjectHeaderMock).toHaveBeenCalledWith(
      expect.objectContaining({
        onSelectProject: expect.any(Function),
        onAddProject: expect.any(Function),
      }),
    )
  })

  // Regression: moving the sidebar from one side to the other must change only
  // the grid areas. Both expensive subtrees stay in the same DOM nodes, and the
  // pixel preference stays one value rather than inheriting a panel percentage.
  describe('moving the sidebar to the other side', () => {
    function setSide(side: 'left' | 'right') {
      act(() => {
        useSettingsStore.setState((state) => {
          state.settings.sidebarPosition = side
        })
      })
    }

    afterEach(() => setSide('left'))

    it('changes grid placement without changing the preferred pixel width', () => {
      setSide('left')
      render(<IDEShell />)
      const split = document.querySelector('[data-sidebar-split-pane]') as HTMLElement
      const width = split.style.getPropertyValue('--sidebar-track-width')
      expect(split).toHaveAttribute('data-side', 'left')
      expect(split.style.gridTemplateAreas).toBe('"sidebar handle content"')

      setSide('right')
      expect(split).toHaveAttribute('data-side', 'right')
      expect(split.style.gridTemplateAreas).toBe('"content handle sidebar"')
      expect(split.style.getPropertyValue('--sidebar-track-width')).toBe(width)
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
