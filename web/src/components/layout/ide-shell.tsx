import { useState, useRef, useEffect } from 'react'
import { Outlet, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarTabBar } from './sidebar-tab-bar'
import { ContextPill } from './context-pill'
import { SidebarCarousel } from './sidebar-carousel'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
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
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { recordWorkspaceScopeFromPath, setWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
import { dataOf } from '@/lib/loadable'
import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'

const SIDEBAR_MIN_PX = 250
const SIDEBAR_MAX_PX = 640

function loadSidebarWidth(): number {
  try {
    const stored = parseInt(localStorage.getItem('sidebar-width') ?? '', 10)
    return Number.isFinite(stored) ? Math.max(SIDEBAR_MIN_PX, stored) : 294
  } catch {
    return 294
  }
}

export function IDEShell() {
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const repos = useSidebarStore((s) => s.repos)
  // Feeds the sidebar header's back/forward arrows. Mounted here rather than in
  // SidebarProjectHeader so history keeps accruing while the header is hidden
  // (nav screens) and survives the header unmounting.
  useNavigationHistory()
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const sidebarPanelRef = useRef<PanelImperativeHandle | null>(null)

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
  const activeRepo = repos.find((r) => r.id === activeRepoIdFromRoute)
  // Use the on-disk worktree path from the backend DTO so that "Copy Path" and
  // other filesystem operations produce real paths regardless of how workspaces
  // are created. Non-default workspaces expose localPath on their WorkspaceDTO;
  // the default (main-worktree) workspace falls back to the repo's root path
  // (RepoDTO.path), which is the same directory.
  const activeWorkspace = activeRepo?.workspaces.find((w) => w.id === activeWorkspaceId)
  // For the home route there is no repoId, so fall back to any repo under the
  // active project, then to the project's own path (the home workspace root).
  const allProjects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const projectFallbackPath = homeRouteMatch
    ? (repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
      allProjects.find((p) => p.id === activeProjectIdFromRoute)?.path ??
      '')
    : ''
  const activeWorkspaceRepoPath =
    activeWorkspace?.localPath ?? activeRepo?.localPath ?? projectFallbackPath

  const hasNavScreen = useSidebarNavStore((s) => s.stack.length > 0)

  // BUG-003: when landing directly on a workspace route, the header project
  // button showed "Select project" — the active project was never derived from
  // the route. Keep the active project in sync with the route's projectId.
  const workspaceProjectId = activeProjectIdFromRoute ?? activeRepo?.projectId
  useEffect(() => {
    if (!workspaceProjectId) return
    if (useProjectStore.getState().activeProjectId !== workspaceProjectId) {
      useProjectStore.getState().setActiveProject(workspaceProjectId)
    }
  }, [workspaceProjectId])

  // Drive panel collapse/expand from sidebarOpen state (set by SidebarProvider's toggleSidebar)
  useEffect(() => {
    const panel = sidebarPanelRef.current
    if (!panel) return
    if (sidebarOpen) {
      panel.expand()
    } else {
      panel.collapse()
    }
  }, [sidebarOpen])

  function handleSidebarResize(size: PanelSize) {
    const isCollapsed = size.asPercentage === 0
    setSidebarOpen((prev) => (!isCollapsed !== prev ? !isCollapsed : prev))
    if (size.inPixels > 0) {
      try {
        localStorage.setItem('sidebar-width', String(Math.round(size.inPixels)))
      } catch {
        // storage unavailable
      }
    }
  }

  const sidebarContent = (
    <div className="relative flex h-full flex-col overflow-hidden bg-transparent select-none">
      {!hasNavScreen && <SidebarProjectHeader />}
      {!hasNavScreen && <ContextPill />}
      {!hasNavScreen && <SidebarTabBar />}
      <ErrorBoundary>
        <SidebarCarousel activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
      </ErrorBoundary>
      <SidebarToastOverlay sidebarOpen={sidebarOpen} sidebarSide={sidebarPosition ?? 'left'} />
    </div>
  )

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
      <ResizablePanelGroup
        orientation="horizontal"
        className="h-full w-full"
        onLayoutChange={() => {
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }}
        onLayoutChanged={() => {
          document.documentElement.removeAttribute('data-pane-resizing')
          window.dispatchEvent(new CustomEvent('pane-resize-end'))
        }}
      >
        {sidebarPosition === 'right' ? (
          <>
            <ResizablePanel minSize="20%" className="min-w-0">
              {contentEl}
            </ResizablePanel>
            <ResizableHandle data-testid="sidebar-resize-handle" />
            <ResizablePanel
              ref={sidebarPanelRef}
              collapsible
              defaultSize={loadSidebarWidth()}
              minSize={SIDEBAR_MIN_PX}
              maxSize={SIDEBAR_MAX_PX}
              collapsedSize={0}
              groupResizeBehavior="preserve-pixel-size"
              onResize={handleSidebarResize}
            >
              {sidebarContent}
            </ResizablePanel>
          </>
        ) : (
          <>
            <ResizablePanel
              ref={sidebarPanelRef}
              collapsible
              defaultSize={loadSidebarWidth()}
              minSize={SIDEBAR_MIN_PX}
              maxSize={SIDEBAR_MAX_PX}
              collapsedSize={0}
              groupResizeBehavior="preserve-pixel-size"
              onResize={handleSidebarResize}
            >
              {sidebarContent}
            </ResizablePanel>
            <ResizableHandle data-testid="sidebar-resize-handle" />
            <ResizablePanel minSize="20%" className="min-w-0">
              {contentEl}
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      <SettingsDialog
        isOpen={isSettingsOpen}
        onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
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
