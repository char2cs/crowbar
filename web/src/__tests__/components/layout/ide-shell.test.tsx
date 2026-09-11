// web/src/__tests__/components/layout/IDEShell.test.tsx
import { act, render, screen } from '@testing-library/react'
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/ide-shell'
import { useSettingsStore } from '@/features/settings/store'
import { useProjectDataStore } from '@/lib/store/projects'
import { idle, success } from '@/lib/loadable'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const router = vi.hoisted(() => ({ pathname: '/', navigate: vi.fn() }))

vi.mock('@/utils/platform', () => ({
  IS_MAC: true,
  IS_WINDOWS: false,
  IS_LINUX: false,
}))
const { workspaceViewMock } = vi.hoisted(() => ({
  workspaceViewMock: vi.fn(),
}))
vi.mock('@/features/workspace/components/workspace-view', () => ({
  WorkspaceView: (props: { wsId: string; active: boolean }) => {
    workspaceViewMock(props)
    return <div data-testid="workspace-view" />
  },
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
const { sidebarFooterMock } = vi.hoisted(() => ({
  sidebarFooterMock: vi.fn(),
}))
vi.mock('@/components/layout/sidebar-footer', () => ({
  SidebarFooter: (props: unknown) => {
    sidebarFooterMock(props)
    return <div data-testid="sidebar-footer" />
  },
}))
vi.mock('@/features/settings/components/settings-dialog', () => ({
  default: () => null,
}))
// `repos` is mutable (not frozen at mock-definition time) so tests can seed
// real workspace/chat data for the active-pane-resolution tests below —
// every other test leaves it at the empty default.
const { sidebarState } = vi.hoisted(() => ({
  sidebarState: {
    repos: [] as unknown[],
    collapsedRepos: new Set(),
    deleteWorkspace: vi.fn(),
    toggleRepo: vi.fn(),
  },
}))
vi.mock('@/lib/store/sidebar', () => {
  return {
    useSidebarStore: (selector?: (s: typeof sidebarState) => unknown) =>
      selector ? selector(sidebarState) : sidebarState,
    // Module-level sentinels home-tree.ts needs at import time (transitively
    // reached via use-workspace-provider-stream -> sidebar-sync ->
    // project-visibility -> home-tree) — not read by anything this test
    // exercises, but a mocked module without them throws on load.
    EMPTY_CHATS: [],
    EMPTY_FOLDERS: [],
  }
})
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => router.navigate,
  useRouterState: (opts?: { select?: (s: { location: { pathname: string } }) => unknown }) => {
    const state = { location: { pathname: router.pathname } }
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
  beforeEach(() => {
    router.pathname = '/'
    router.navigate = vi.fn()
    useProjectDataStore.setState({ data: idle() })
    sidebarState.repos = []
    workspaceViewMock.mockClear()
    // A fresh windowPaneStore starts on the empty-stage fallback pane (no
    // chat, no editor tabs) — which now hides SidebarCarousel (see "hiding
    // with nothing open" below). Every OTHER test here is about chrome that
    // only makes sense alongside a real view, so it needs one seeded in.
    resetWindowPaneStoreForTests()
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'test-chat', null)
  })

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

  // The product owner asked the project marks back at the sidebar's true
  // bottom, below the floating file-explorer card — out of
  // SidebarProjectHeader's cramped window-chrome row, where an earlier pass
  // had put them. SidebarProjectHeader now takes no project-related props at
  // all; SidebarFooter mounts as its own sibling row instead.
  it('renders SidebarProjectHeader with no project-related props', () => {
    render(<IDEShell />)
    expect(sidebarProjectHeaderMock).toHaveBeenCalledWith({})
  })

  it('mounts SidebarFooter below the carousel, wired to the same project data and handlers', () => {
    render(<IDEShell />)
    const footer = screen.getByTestId('sidebar-footer')
    const carousel = screen.getByTestId('sidebar-carousel')
    // DOCUMENT_POSITION_FOLLOWING: the footer comes after the carousel in DOM order.
    expect(carousel.compareDocumentPosition(footer)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
    expect(sidebarFooterMock).toHaveBeenCalledWith(
      expect.objectContaining({
        projects: [],
        activeProjectId: undefined,
        onSelectProject: expect.any(Function),
        onAddProject: expect.any(Function),
      }),
    )
  })

  // Regression: SidebarCarousel floats absolutely over whatever
  // SidebarTreeSurface renders (its own doc comment) — over the create-space
  // form that meant it covered the Cancel button. Hidden entirely while
  // creating a space, not just made to respect a z-index of its own.
  describe('creating a space', () => {
    function lastFooterProps() {
      const calls = sidebarFooterMock.mock.calls
      return calls[calls.length - 1][0] as {
        onAddProject: () => void
        onSelectProject: (id: string) => void
      }
    }

    it('hides SidebarCarousel while the create-space form is open, restores it on cancel', () => {
      render(<IDEShell />)
      expect(screen.getByTestId('sidebar-carousel')).toBeInTheDocument()

      act(() => lastFooterProps().onAddProject())
      expect(screen.queryByTestId('sidebar-carousel')).not.toBeInTheDocument()
    })

    // Picking an existing space while the create form is open has to replace
    // it, not leave the form stranded behind the newly-routed content.
    it('selecting an existing space closes the create-space form', () => {
      render(<IDEShell />)
      act(() => lastFooterProps().onAddProject())
      expect(screen.queryByTestId('sidebar-carousel')).not.toBeInTheDocument()

      act(() => lastFooterProps().onSelectProject('p1'))
      expect(screen.getByTestId('sidebar-carousel')).toBeInTheDocument()
    })
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

  // Regression: deleting the project you're sitting on (or having it vanish
  // any other way — a teammate's delete, a live WS reseed) used to leave you
  // stuck on its now-dead route, "Project Home unavailable" on screen with
  // the gone project's own sidebar still drawn behind it. `leaveIfRemoved`
  // (removal-commit.ts) only ever navigates away for a draining
  // 'workspace'/'folder'/'chat' hold — a 'repo'/'project' hold's `hiddenIds`
  // names repo/project ids, never a workspace id, so it silently did nothing
  // for exactly the two removals that can strand the user like this. This is
  // the general invariant that makes that state unreachable regardless of
  // what caused it.
  describe('the routed project no longer exists', () => {
    it('leaves for "/" once the live project list loads without it', () => {
      router.pathname = '/ide/p1/home'
      useProjectDataStore.setState({
        data: success([{ id: 'p2', name: 'other', path: '/p2', lastActivity: new Date(0) }]),
      })

      render(<IDEShell />)

      expect(router.navigate).toHaveBeenCalledWith({ to: '/' })
    })

    it('stays put while the project list is still loading', () => {
      router.pathname = '/ide/p1/home'
      useProjectDataStore.setState({ data: idle() })

      render(<IDEShell />)

      expect(router.navigate).not.toHaveBeenCalled()
    })

    it('stays put once the routed project is actually in the list', () => {
      router.pathname = '/ide/p1/home'
      useProjectDataStore.setState({
        data: success([{ id: 'p1', name: 'crowbar', path: '/p1', lastActivity: new Date(0) }]),
      })

      render(<IDEShell />)

      expect(router.navigate).not.toHaveBeenCalled()
    })
  })

  // The Files/Git card only makes sense alongside a real view — with nothing
  // open (the empty-stage fallback pane, spec §5.4's tumbling wordmark) there
  // is no workspace in particular it would be showing.
  describe('hiding with nothing open', () => {
    it('hides SidebarCarousel while the showing tree is only the empty-stage pane', () => {
      resetWindowPaneStoreForTests()

      render(<IDEShell />)

      expect(screen.queryByTestId('sidebar-carousel')).not.toBeInTheDocument()
    })

    it('shows SidebarCarousel once a real chat is open', () => {
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'test-chat', null)

      render(<IDEShell />)

      expect(screen.getByTestId('sidebar-carousel')).toBeInTheDocument()
    })
  })

  // Regression: clicking into a different pane of a split never changed
  // `effectiveActiveWorkspaceId` (route-only), so WorkspaceHost kept exactly
  // the routed workspace "active" — the file-system/git stores and the
  // pane/save keyboard use-workspace-effects.ts mounts only for that one
  // workspace never followed. Caught live: two panes on totally different
  // repos, and the file explorer stayed on whichever repo the URL happened
  // to name, regardless of which pane you clicked into.
  describe('the active pane, not the route, decides WorkspaceHost\'s active workspace', () => {
    it("mounts the active pane's own workspace as active, even for a repo the route never visited", () => {
      router.pathname = '/ide/p1/r1/ws-a'
      sidebarState.repos = [
        {
          id: 'r1',
          projectId: 'p1',
          workspaces: [{ id: 'ws-a', localPath: '/repo-a' }],
          chats: [{ id: 'chat-a', workspaceId: 'ws-a' }],
        },
        {
          id: 'r2',
          projectId: 'p1',
          workspaces: [{ id: 'ws-b', localPath: '/repo-b' }],
          chats: [{ id: 'chat-b', workspaceId: 'ws-b' }],
        },
      ]
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-a', null)
      const secondPaneId = windowPaneStore
        .getState()
        .paneActions.splitPane(ROOT_PANE_ID, 'horizontal', undefined, 'after')
      windowPaneStore.getState().paneActions.setPaneChat(secondPaneId!, 'chat-b', null)
      windowPaneStore.getState().paneActions.setActivePane(secondPaneId!)

      render(<IDEShell />)

      const activeCall = workspaceViewMock.mock.calls
        .map(([props]) => props as { wsId: string; active: boolean })
        .find((p) => p.active)
      expect(activeCall?.wsId).toBe('ws-b')
    })

    it('falls back to the routed workspace while the active pane has no chat of its own', () => {
      router.pathname = '/ide/p1/r1/ws-a'
      sidebarState.repos = [
        {
          id: 'r1',
          projectId: 'p1',
          workspaces: [{ id: 'ws-a', localPath: '/repo-a' }],
          chats: [{ id: 'chat-a', workspaceId: 'ws-a' }],
        },
      ]
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-a', null)

      render(<IDEShell />)

      const activeCall = workspaceViewMock.mock.calls
        .map(([props]) => props as { wsId: string; active: boolean })
        .find((p) => p.active)
      expect(activeCall?.wsId).toBe('ws-a')
    })
  })
})
