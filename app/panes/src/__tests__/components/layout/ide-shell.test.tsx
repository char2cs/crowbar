import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { IDEShell } from '@/components/layout/ide-shell'
import { useSettingsStore } from '@/features/settings/store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { workspaceRef } from '@/lib/api/documents'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import * as sidebarBody from '@/components/layout/sidebar-body'
import * as removalTray from '@/components/layout/removal-tray'
import * as chatWorkspaceId from '@/features/panes/hooks/use-chat-workspace-id'
import {
  resetEntities,
  seedHome,
  seedProjects,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

const router = vi.hoisted(() => ({ pathname: '/', navigate: vi.fn() }))

// WorkspaceView and SidebarTreeSurface are memo objects, so they are replaced at the module.
const { workspaceViewMock } = vi.hoisted(() => ({ workspaceViewMock: vi.fn() }))
vi.mock('@/features/workspace/components/workspace-view', () => ({
  WorkspaceView: (props: { wsId: string; active: boolean }) => {
    workspaceViewMock(props)
    return <div data-testid="workspace-view" />
  },
}))
vi.mock('@/components/layout/sidebar-tree-surface', () => ({
  SidebarTreeSurface: ({ creatingSpace }: { creatingSpace: boolean }) => (
    <div data-testid="sidebar-tree-surface" data-creating-space={String(creatingSpace)} />
  ),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => router.navigate,
  useRouterState: (opts?: { select?: (s: { location: { pathname: string } }) => unknown }) => {
    const state = { location: { pathname: router.pathname } }
    return opts?.select ? opts.select(state) : state
  },
  Outlet: () => <div data-testid="outlet" />,
}))

// Counts shell renders: SidebarBody is a plain child, so it re-renders exactly when the shell does.
const shellRenders = vi.fn()

// The shell's children are markers: this suite is about WHERE the shell mounts them, and each
// child's own test file owns what it does.
function stubShellChildren() {
  vi.spyOn(sidebarBody, 'SidebarBody').mockImplementation(({ chatsPanel, projectSwitcher }) => {
    shellRenders()
    return (
      <div data-testid="sidebar-body">
        {projectSwitcher}
        {chatsPanel}
      </div>
    )
  })
  vi.spyOn(removalTray, 'RemovalTray').mockImplementation(() => <div data-testid="removal-tray" />)
}

describe('IDEShell', () => {
  beforeEach(() => {
    stubShellChildren()
    router.pathname = '/'
    router.navigate = vi.fn()
    resetEntities()
    workspaceViewMock.mockClear()
    shellRenders.mockClear()
    treeUiStore.setState({ activeTab: 'files' })
    // A fresh windowPaneStore starts on the empty-stage fallback pane (no
    // chat, no editor tabs); most tests here want a real view seeded in.
    resetWindowPaneStoreForTests()
    windowPaneStore.getState().paneActions.openChat('test-chat')
  })

  it('renders project header', () => {
    render(<IDEShell />)
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })

  it('renders the sidebar body', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-body')).toBeInTheDocument()
  })

  it("renders SidebarTreeSurface (SpaceScroller's real mount point) as the body's Chats panel", () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-body')).toContainElement(
      screen.getByTestId('sidebar-tree-surface'),
    )
  })

  it('renders Outlet when no workspace is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('renders a resize handle inside the sidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-resize-handle')).toBeInTheDocument()
  })

  it('feeds the project switcher the live projects', () => {
    router.pathname = '/ide/p1/home'
    seedProjects([
      { id: 'p1', name: 'crowbar', path: '/p1', lastActivity: new Date(0), hostId: 'local' },
      { id: 'p2', name: 'other', path: '/p2', lastActivity: new Date(0), hostId: 'local' },
    ])
    render(<IDEShell />)
    expect(screen.getByTestId('active-project-name')).toHaveTextContent('crowbar')
    expect(screen.getByRole('button', { name: 'other' })).toBeInTheDocument()
  })

  // The create-space form lives in the Chats panel (SidebarTreeSurface).
  describe('creating a space', () => {
    const addSpace = () => act(() => screen.getByRole('button', { name: 'Add space' }).click())

    it('switches the sidebar to Chats so the form is on screen', () => {
      render(<IDEShell />)
      addSpace()
      expect(treeUiStore.getState().activeTab).toBe('chats')
      expect(screen.getByTestId('sidebar-tree-surface')).toHaveAttribute(
        'data-creating-space',
        'true',
      )
    })

    // Picking an existing space while the create form is open has to replace
    // it, not leave the form stranded behind the newly-routed content.
    it('selecting an existing space closes the create-space form', () => {
      router.pathname = '/ide/p1/home'
      seedProjects([
        { id: 'p1', name: 'crowbar', path: '/p1', lastActivity: new Date(0), hostId: 'local' },
        { id: 'p2', name: 'other', path: '/p2', lastActivity: new Date(0), hostId: 'local' },
      ])
      render(<IDEShell />)
      addSpace()
      act(() => screen.getByRole('button', { name: 'other' }).click())
      expect(screen.getByTestId('sidebar-tree-surface')).toHaveAttribute(
        'data-creating-space',
        'false',
      )
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
      const sidebarBefore = screen.getByTestId('sidebar-body')
      const contentBefore = screen.getByTestId('outlet')

      setSide('right')

      // Same DOM nodes, reordered. A positional reconcile would have unmounted
      // both subtrees — WorkspaceHost, its terminals and Monaco models included —
      // and built fresh ones.
      expect(screen.getByTestId('sidebar-body')).toBe(sidebarBefore)
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
      seedProjects([
        { id: 'p2', name: 'other', path: '/p2', lastActivity: new Date(0), hostId: 'local' },
      ])

      render(<IDEShell />)

      expect(router.navigate).toHaveBeenCalledWith({ to: '/' })
    })

    it('stays put while the project list is still loading', () => {
      router.pathname = '/ide/p1/home'
      resetEntities()

      render(<IDEShell />)

      expect(router.navigate).not.toHaveBeenCalled()
    })

    it('stays put once the routed project is actually in the list', () => {
      router.pathname = '/ide/p1/home'
      seedProjects([
        { id: 'p1', name: 'crowbar', path: '/p1', lastActivity: new Date(0), hostId: 'local' },
      ])

      render(<IDEShell />)

      expect(router.navigate).not.toHaveBeenCalled()
    })
  })

  // The mode bar and its Chats panel stay usable with nothing open.
  it('renders the sidebar body on the empty stage', () => {
    resetWindowPaneStoreForTests()

    render(<IDEShell />)

    expect(screen.getByTestId('sidebar-body')).toBeInTheDocument()
  })

  /**
   * Live-reported: "I can't delete rows. It starts the timer, and then it
   * never deletes them."
   *
   * `RemovalTray` draws no held row — every kind transforms in place in the
   * tree (sidebar-row.tsx), and the hairline is a CSS animation. What the
   * tray owns is the only 8s commit clock, the seconds numerals, the
   * pagehide flush and `RemovalConfirmDialog`. It was once mounted inside a
   * panel the shell gated on an empty stage and while creating a space, and
   * the gate took the clock with it: no DELETE was ever issued.
   *
   * removal-tray.test.tsx could never catch this — its harness mounts
   * `<RemovalTray />` as a sibling by hand. This asserts the REAL mount
   * point, in the two states that used to remove it.
   */
  describe('the removal clock survives every state the sidebar has', () => {
    it('keeps RemovalTray mounted with nothing open', () => {
      resetWindowPaneStoreForTests()

      render(<IDEShell />)

      expect(screen.getByTestId('removal-tray')).toBeInTheDocument()
    })

    it('keeps RemovalTray mounted while the create-space form is open', () => {
      render(<IDEShell />)

      act(() => screen.getByRole('button', { name: 'Add space' }).click())

      expect(screen.getByTestId('removal-tray')).toBeInTheDocument()
    })
  })

  // Regression: clicking into a different pane of a split never changed
  // `effectiveActiveWorkspaceId` (route-only), so WorkspaceHost kept exactly
  // the routed workspace "active" — the file-system/git stores and the
  // pane/save keyboard use-workspace-effects.ts mounts only for that one
  // workspace never followed. Caught live: two panes on totally different
  // repos, and the file explorer stayed on whichever repo the URL happened
  // to name, regardless of which pane you clicked into.
  describe("the active pane, not the route, decides WorkspaceHost's active workspace", () => {
    it("mounts the active pane's own workspace as active, even for a repo the route never visited", () => {
      router.pathname = '/ide/p1/r1/ws-a'
      seedRepos([
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
      ] as never)
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.openChat('chat-a', { workspaceId: 'ws-a' })
      windowPaneStore.getState().paneActions.dropChatOnPane('chat-b', ROOT_PANE_ID, 'right', 'ws-b')
      const secondPaneId = chatPaneIndex(windowPaneStore.getState().panes).get('chat-b')
      windowPaneStore.getState().paneActions.setActivePane(secondPaneId!)

      render(<IDEShell />)

      const activeCall = workspaceViewMock.mock.calls
        .map(([props]) => props as { wsId: string; active: boolean })
        .find((p) => p.active)
      expect(activeCall?.wsId).toBe('ws-b')
    })

    /**
     * ...and moving focus WITHIN one workspace's view must cost nothing.
     *
     * `useNavigationHistory` renders nothing — it only records into the jump
     * list — but it was called in IDEShell's own body while subscribing to
     * `panes[activePaneId].activeEditorTabId`, which moves whenever focus
     * crosses between a pane holding an editor tab and one that doesn't. That
     * re-rendered the app's root, and nothing below it is memoized: measured
     * live with three panes tiled in one workspace view, ~810 fibers per click
     * — the whole sidebar, every row's tooltip and dropdown, the
     * file-explorer card — for output that never changed, dropping 120fps to
     * 78. The subscription now lives in a childless `NavigationHistoryRecorder`
     * leaf, so the shell itself sees nothing.
     *
     * `SidebarProjectHeader` is the probe: it takes no props and is not
     * memoized, so it re-renders exactly when IDEShell does.
     */
    it('does not re-render the shell when focus moves between panes of one workspace', () => {
      router.pathname = '/ide/p1/r1/ws-a'
      seedRepos([
        {
          id: 'r1',
          projectId: 'p1',
          workspaces: [{ id: 'ws-a', localPath: '/repo-a' }],
          // BOTH chats in the SAME workspace — this is the within-one-view
          // case, so `useActivePaneWorkspaceId` holds still throughout and the
          // only thing that moves is which pane is focused.
          chats: [
            { id: 'chat-a', workspaceId: 'ws-a' },
            { id: 'chat-b', workspaceId: 'ws-a' },
          ],
        },
      ] as never)
      resetWindowPaneStoreForTests()
      const paneActions = () => windowPaneStore.getState().paneActions
      paneActions().openChat('chat-a', { workspaceId: 'ws-a' })
      paneActions().dropChatOnPane('chat-b', ROOT_PANE_ID, 'right', 'ws-a')
      const secondPaneId = chatPaneIndex(windowPaneStore.getState().panes).get('chat-b')!
      // One pane holds an editor tab, the other only its chat: the shell must not
      // re-render on an active-editor-tab change it does not read.
      paneActions().setActivePane(ROOT_PANE_ID)
      windowPaneStore.getState().bufferActions.openContent({
        type: 'editor',
        doc: workspaceRef('ws-a', '/repo-a/a.ts'),
        name: 'a.ts',
      })

      render(<IDEShell />)
      const rendersBefore = shellRenders.mock.calls.length

      act(() => paneActions().setActivePane(secondPaneId))
      act(() => paneActions().setActivePane(ROOT_PANE_ID))

      expect(shellRenders.mock.calls.length).toBe(rendersBefore)
    })

    it('falls back to the routed workspace while the active pane has no chat of its own', () => {
      router.pathname = '/ide/p1/r1/ws-a'
      seedRepos([
        {
          id: 'r1',
          projectId: 'p1',
          workspaces: [{ id: 'ws-a', localPath: '/repo-a' }],
          chats: [{ id: 'chat-a', workspaceId: 'ws-a' }],
        },
      ] as never)
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.openChat('chat-a', { workspaceId: 'ws-a' })

      render(<IDEShell />)

      const activeCall = workspaceViewMock.mock.calls
        .map(([props]) => props as { wsId: string; active: boolean })
        .find((p) => p.active)
      expect(activeCall?.wsId).toBe('ws-a')
    })
  })

  // Live-reported: "on a split view, the file explorer of a thread doesn't
  // appear... only when in a view with threads AND branch threads opened."
  // The active pane holds a project-home chat (rides no repo — home is a
  // project-level concept per home-workspace-resolver.ts), while the ROUTE
  // sits on a repo workspace of the SAME project (a branch thread in the
  // same split). `ide-shell.tsx` used to resolve `homeWorkspaceId`/
  // `homeWorkspacePath` only when the ROUTE itself was on the home path
  // (`homeProjectId = homeRouteMatch ? activeProjectIdFromRoute : undefined`),
  // so off the home route they stayed undefined even though the active pane's
  // own workspace WAS the home one — sending the sidebar's path lookup into
  // `use-ide-shell-workspace-retention.ts`'s `!isHomeRoute` dead end.
  describe('mixed split: a project-home pane active while the route sits on a repo workspace', () => {
    it("resolves the sidebar's file-explorer path to the HOME workspace's own path, not the empty state", async () => {
      router.pathname = '/ide/p1/r1/ws-a'
      seedRepos([
        {
          id: 'r1',
          projectId: 'p1',
          localPath: '/repo-a',
          workspaces: [{ id: 'ws-a', localPath: '/repo-a' }],
          // The branch thread sharing the split with the project-home chat.
          chats: [{ id: 'chat-a', workspaceId: 'ws-a' }],
        },
      ] as never)
      seedHome('p1', 'ws-home-1', {
        owningChatId: 'home-chat-1',
        localPath: '/Users/mateo/projects/rabbyte-labs',
      })
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.openChat('home-chat-1')
      // A home chat rides no repo, so its pane resolves through a registered workspace store; stand
      // that resolution in (the "active pane, not the route, decides" tests exercise it for real).
      vi.spyOn(chatWorkspaceId, 'useActivePaneWorkspaceId').mockReturnValue('ws-home-1')

      render(<IDEShell />)

      await waitFor(() => {
        expect(useFocusedWorkspaceContextStore.getState().rootPath).toBe(
          '/Users/mateo/projects/rabbyte-labs',
        )
      })
      // Not repo r1's own checkout borrowed as a stand-in for "the project's root".
      const ctx = useFocusedWorkspaceContextStore.getState()
      expect(ctx.rootPath).not.toBe('/repo-a')
      expect(ctx).toMatchObject({ workspaceId: 'ws-home-1', isProjectHome: true, repoId: null })
    })
  })
})
