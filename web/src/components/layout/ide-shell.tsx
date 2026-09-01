import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarCarousel } from './sidebar-carousel'
import { SidebarTreeSurface } from './sidebar-tree-surface'
import { SidebarFooter } from './sidebar-footer'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  useProjectStore,
  useProjectDataStore,
  EMPTY_PROJECTS,
  importProjectAndSync,
} from '@/lib/store/projects'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import type { Project } from '@/lib/types'
import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { ConnectionIndicator } from './connection-indicator'
import { FpsOverlay } from './fps-overlay'
import { DetachHolderModal } from './detach-holder-modal'
import { PlaceholderToastWatcher } from './placeholder-toast-watcher'
import { SidebarToastOverlay } from './sidebar-toast-overlay'
import { SidebarPeek } from './sidebar-peek'
import { useSidebarPanel, SIDEBAR_MIN_PX, SIDEBAR_MAX_PX } from './use-sidebar-panel'
import { SidebarSplitPane } from './sidebar-split-pane'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { recordWorkspaceScopeFromPath, setWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
import { dataOf } from '@/lib/loadable'
import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'

export function IDEShell() {
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const navigate = useNavigate()
  // Feeds the sidebar header's back/forward arrows. Mounted here rather than in
  // SidebarProjectHeader so history keeps accruing while the header is hidden
  // (nav screens) and survives the header unmounting.
  useNavigationHistory()
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const sidebarSide = sidebarPosition === 'right' ? 'right' : 'left'
  const { sidebarOpen, setSidebarOpen, preferredWidth, commitPreferredWidth } = useSidebarPanel()

  // §7: the TanStack /ide/:projectId/:repoId/:wsId route params are the
  // canonical source for the active project/repo/workspace — read them directly
  // rather than scanning the sidebar store (which lags the route on cold start).
  // Recording the scope here, SYNCHRONOUSLY during render, is load-bearing: the
  // WorkspaceView subtree (rendered below) builds workspace-scoped URLs via
  // workspaceBase() during its own render, so the scope must exist before then —
  // recording it only in the route component's post-render effect threw on first
  // paint and tripped the ErrorBoundary (§14 add-repo regression).
  const routeScope = recordWorkspaceScopeFromPath(pathname)
  const homeRouteMatch = routeScope ? null : pathname.match(/\/ide\/([^/]+)\/home$/)
  const activeProjectIdFromRoute = routeScope?.projectId ?? homeRouteMatch?.[1]
  const activeRepoIdFromRoute = routeScope?.repoId
  const activeWorkspaceId = routeScope?.wsId

  // Resolve the home workspace id ONCE per project (cached for the session —
  // see home-workspace-resolver.ts) instead of HomeRoute re-fetching and
  // cold-mounting a fresh WorkspaceView on every single visit to project
  // home. Kicking the fetch off here (not in HomeRoute) means WorkspaceHost
  // below can keep the resulting workspace mounted-but-hidden via its normal
  // keep-alive retention, so a repeat visit is a warm slot reveal.
  const homeProjectId = homeRouteMatch ? activeProjectIdFromRoute : undefined
  const { wsId: homeWorkspaceId } = useHomeWorkspaceState(homeProjectId ?? null)
  useEffect(() => {
    if (homeProjectId) ensureHomeWorkspaceResolved(homeProjectId)
  }, [homeProjectId])
  // Recorded SYNCHRONOUSLY during render (matches recordWorkspaceScopeFromPath
  // above) — WorkspaceHost, rendered below, mounts this workspace's
  // WorkspaceView in the same render pass, and workspaceBase() needs the
  // scope to exist before that.
  if (homeRouteMatch && homeProjectId && homeWorkspaceId) {
    setWorkspaceScope({ projectId: homeProjectId, repoId: '', wsId: homeWorkspaceId })
  }
  // The workspace WorkspaceHost should treat as "active": the routed
  // workspace, or — on project home — the resolved home workspace once known.
  const effectiveActiveWorkspaceId = activeWorkspaceId ?? homeWorkspaceId ?? null
  // Open the per-:wsId workspace WS stream for the viewed workspace. Beyond data,
  // this is what starts the daemon's per-connection provider poll so a branch with
  // an open PR flips to the green pr-open icon (the list stream never starts it).
  useWorkspaceProviderStream(activeProjectIdFromRoute, activeRepoIdFromRoute, activeWorkspaceId)
  // The shell only needs one scalar from the sidebar tree. Subscribing to the
  // whole repos array made every live status/count frame rebuild the complete
  // IDE shell — sidebar provider, carousel, offscreen panels and workspace host
  // included. Returning the resolved path lets Zustand bail out unless the
  // active workspace's actual filesystem scope changed.
  const sidebarWorkspacePath = useSidebarStore((s) => {
    const activeRepo = s.repos.find((r) => r.id === activeRepoIdFromRoute)
    const activeWorkspace = activeRepo?.workspaces.find((w) => w.id === activeWorkspaceId)
    if (activeWorkspace?.localPath) return activeWorkspace.localPath
    if (activeRepo?.localPath) return activeRepo.localPath
    if (!homeRouteMatch) return ''
    return s.repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ?? ''
  })
  // For the home route there is no repoId, so fall back to any repo under the
  // active project, then to the project's own path (the home workspace root).
  const allProjects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  // Space marks (spec §4.1): same navigation the workspace switcher's own
  // project-home rows already use (workspace-switcher.tsx's `select`).
  // Shared verbatim with SidebarTreeSurface's SpaceScroller below
  // (`onActiveProjectChange`) — one "switch to project" action, not two.
  const handleSelectProject = (projectId: string) => {
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }
  // The tree's only entry point for a SECOND project (spec §3 ruling,
  // relocated per user direction to the sidebar's own footer): a trailing
  // `+` mark in SidebarFooter, not a reopened tree-foot row. Lifted here,
  // alongside `allProjects`, so both SidebarFooter (the mark) and this
  // modal can reach it — moved verbatim from the old SidebarTreeChrome-
  // owned state.
  const [importProjectOpen, setImportProjectOpen] = useState(false)
  const handleImportProject = useCallback((project: Project) => {
    importProjectAndSync(project)
    setImportProjectOpen(false)
  }, [])
  const projectFallbackPath = homeRouteMatch
    ? (allProjects.find((p) => p.id === activeProjectIdFromRoute)?.path ?? '')
    : ''
  const activeWorkspaceRepoPath = sidebarWorkspacePath || projectFallbackPath

  const hasNavScreen = useSidebarNavStore((s) => s.stack.length > 0)

  // The floating file-explorer card's own rail (spec §6: it "opens at one
  // third of the sidebar's height" and "height is kept as a proportion of
  // the rail"). Measured here, not inside SidebarCarousel, because the rail
  // IS this column — measure synchronously on mount (mirrors
  // use-tab-bar-scroll.ts's own layout-effect + ResizeObserver pattern) so
  // the card opens at the right height on first paint, not one frame late.
  // The rail is a `flex-1` child of the outer sidebar column below, not the
  // whole column itself (task-10): SidebarFooter is a SIBLING after it, in
  // its own reserved `flex-none` space, so the rail's measured height
  // already excludes the footer — the card's own bottom-inset math (still
  // relative to THIS element) needs no change to account for it.
  const sidebarRailRef = useRef<HTMLDivElement>(null)
  const [sidebarRailHeight, setSidebarRailHeight] = useState(0)
  useLayoutEffect(() => {
    const el = sidebarRailRef.current
    if (!el) return
    const measure = () => setSidebarRailHeight(el.getBoundingClientRect().height)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  // The tree's own bottom inset (spec §6) no longer flows through React
  // state or props here: SidebarCarousel writes its live height straight
  // onto `sidebarRailRef` as a `--card-bottom-inset` CSS custom property
  // (via the `railRef` prop below), and `space-scroller.tsx`'s `SpacePanel`
  // reads it back through plain CSS inheritance. A per-frame React state
  // update here previously re-rendered this whole shell — and, since none
  // of SidebarTreeSurface/SpaceScroller/SpacePanel/SidebarTree/SidebarRow
  // are memoized, every visible row — on every frame of a resize drag.

  // BUG-003: when landing directly on a workspace route, the header project
  // button showed "Select project" — the active project was never derived from
  // the route. Keep the active project in sync with the route's projectId.
  const workspaceProjectId = activeProjectIdFromRoute
  useEffect(() => {
    if (!workspaceProjectId) return
    if (useProjectStore.getState().activeProjectId !== workspaceProjectId) {
      useProjectStore.getState().setActiveProject(workspaceProjectId)
    }
  }, [workspaceProjectId])

  // SidebarPeek is a wrapper, not a branch: it renders in every state and only
  // restyles itself, so hiding the sidebar never rebuilds the subtree below it.
  //
  // The rail (header/tree/floating card/toasts) and SidebarFooter are two
  // `flex` siblings in an outer column, not the rail's own children (task-10:
  // the user's explicit override of spec §4.1's window-chrome placement —
  // "the last element on the sidebar, right down [from] the file explorer,
  // but not inside it, just outside, at the end of the sidebar"). Putting the
  // footer OUTSIDE `sidebarRailRef` — rather than appending it inside as one
  // more flow child — means the rail (still the ONLY element the floating
  // card's `absolute inset-x-2 bottom-2` is positioned against, and the only
  // element `sidebarRailHeight` measures) simply gets shorter by the
  // footer's own reserved `flex-none` height. The card's own resize/float
  // math in sidebar-carousel.tsx needed no changes at all.
  const sidebarContent = (
    <SidebarPeek hidden={!sidebarOpen} side={sidebarSide} width={preferredWidth}>
      <div className="flex h-full min-h-0 flex-col bg-transparent select-none">
        <div
          ref={sidebarRailRef}
          className="relative flex min-h-0 flex-1 flex-col overflow-hidden"
        >
          {!hasNavScreen && <SidebarProjectHeader />}
          {!hasNavScreen && (
            <SidebarTreeSurface
              projects={allProjects}
              activeProjectId={activeProjectIdFromRoute}
              onActiveProjectChange={handleSelectProject}
            />
          )}
          <ErrorBoundary>
            <SidebarCarousel
              activeWorkspaceRepoPath={activeWorkspaceRepoPath}
              sidebarHeight={sidebarRailHeight}
              railRef={sidebarRailRef}
            />
          </ErrorBoundary>
          <SidebarToastOverlay sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
        </div>
        {/* The sidebar's true last element (task-10) — a sibling AFTER the
            rail above (which itself ends with the floating file-explorer
            card), never inside it. */}
        <SidebarFooter
          projects={allProjects}
          activeProjectId={activeProjectIdFromRoute}
          onSelectProject={handleSelectProject}
          onAddProject={() => setImportProjectOpen(true)}
        />
      </div>
    </SidebarPeek>
  )

  // Every pane in here is its OWN drop target now (PaneContainer spreads
  // `PANE_DROP_ATTR` per pane, keyed by pane id) — spec §8.1's four-target
  // table, not the one whole-content "drop anywhere in here to remove it"
  // zone this div used to be (Task 22 deleted that dwell-to-remove gesture
  // along with `editor-removal-overlay.tsx`).
  const contentEl = (
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      <ErrorBoundary>
        {/* WorkspaceHost stays mounted for the whole IDE session — including on
            the project-home route. Unmounting the host on every home visit
            destroyed all keep-alive retention (stores, terminals, Monaco
            models) — so returning to a workspace was a full COLD re-mount
            every time. Keeping the host mounted lets it retain
            recently-visited workspaces (all hidden) across home transits, so
            the return is warm.

            On the home route, `effectiveActiveWorkspaceId` is the resolved
            home workspace (once known) — the host renders ITS WorkspaceView
            too, as just another retained slot, instead of HomeRoute
            cold-mounting a fresh one on every visit (that used to be ~2x the
            frame cost of a normal warm switch; see
            home-workspace-resolver.ts). `homeWsIds` protects every home
            workspace resolved so far this session from the existence-prune —
            home is a project-level concept, not in the sidebar's repo/
            workspace id set, so without this it would look "closed" the
            instant it goes hidden and get destroyed instead of retained.
            HomeRoute itself renders null (or the error state); the Outlet
            still stays mounted so workspace-route components' route-level
            guards keep running. */}
        <WorkspaceHost
          activeWsId={effectiveActiveWorkspaceId}
          homeWsIds={getKnownHomeWorkspaceIds()}
        />
        <Outlet />
      </ErrorBoundary>
    </div>
  )

  return (
    <SidebarProvider
      className="h-screen bg-transparent text-foreground"
      open={sidebarOpen}
      onOpenChange={setSidebarOpen}
    >
      {/* Grid areas move the two already-mounted regions when the side changes;
          neither the sidebar tree nor WorkspaceHost is reconciled into a new
          position. The separator owns pointer tracking only while it is being
          dragged, so ordinary pointer movement over either region has no split
          layout work to do. */}
      <SidebarSplitPane
        side={sidebarSide}
        open={sidebarOpen}
        preferredWidth={preferredWidth}
        minWidth={SIDEBAR_MIN_PX}
        maxWidth={SIDEBAR_MAX_PX}
        sidebar={sidebarContent}
        onOpenChange={setSidebarOpen}
        onWidthCommit={commitPreferredWidth}
      >
        {contentEl}
      </SidebarSplitPane>
      <SettingsDialog
        isOpen={isSettingsOpen}
        onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
      />
      <ImportProjectModal
        open={importProjectOpen}
        onOpenChange={setImportProjectOpen}
        onImport={handleImportProject}
      />
      <TerminalHost />
      <FontStyleInjector />
      <ConnectionIndicator />
      <FpsOverlay />
      <DetachHolderModal />
      <PlaceholderToastWatcher />
    </SidebarProvider>
  )
}
