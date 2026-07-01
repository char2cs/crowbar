import { useState, useRef, useEffect, useLayoutEffect } from 'react'
import { Outlet, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
import { SidebarProjectHeader } from './sidebar-project-header'
import { SidebarTabBar } from './sidebar-tab-bar'
import { ContextPill } from './context-pill'
import { SidebarCarousel } from './sidebar-carousel'
import { IS_MAC } from '@/utils/platform'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { ConnectionIndicator } from './connection-indicator'
import { FpsOverlay } from './fps-overlay'
import { DetachHolderModal } from './detach-holder-modal'
import { PlaceholderToastWatcher } from './placeholder-toast-watcher'
import { SidebarToastOverlay } from './sidebar-toast-overlay'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { recordWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
import { dataOf } from '@/lib/loadable'

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
  const chats = useSidebarStore((s) => s.chats)
  const repos = useSidebarStore((s) => s.repos)
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
  // Open the per-:wsId workspace WS stream for the viewed workspace. Beyond data,
  // this is what starts the daemon's per-connection provider poll so a branch with
  // an open PR flips to the green pr-open icon (the list stream never starts it).
  useWorkspaceProviderStream(activeProjectIdFromRoute, activeRepoIdFromRoute, activeWorkspaceId)
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]
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
    ? (repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath
        ?? allProjects.find((p) => p.id === activeProjectIdFromRoute)?.path
        ?? '')
    : ''
  const activeWorkspaceRepoPath =
    activeWorkspace?.localPath ?? activeRepo?.localPath ?? projectFallbackPath
  const chatTabLabel = chats.find((c) => c.id === activeChatId)?.title ?? 'Chat'

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

  // Signal to root ToastProvider that IDEShell is mounted so it can suppress
  // the fixed-position global toast overlay (SidebarToastOverlay takes over).
  // useLayoutEffect fires synchronously before paint so ideShellMounted is true
  // before the root ever renders <Toasts>, preventing a double-viewport flash.
  useLayoutEffect(() => {
    useUIState.getState().setIdeShellMounted(true)
    return () => useUIState.getState().setIdeShellMounted(false)
  }, [])

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
      <SidebarToastOverlay
        sidebarOpen={sidebarOpen}
        sidebarSide={sidebarPosition ?? 'left'}
      />
    </div>
  )

  const contentEl = (
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      <ErrorBoundary>
        {activeWorkspaceId ? (
          <>
            <WorkspaceView wsId={activeWorkspaceId} />
            {/* Route components render null UI but carry route-level guards
                (e.g. unknown-workspace redirect); they must stay mounted. */}
            <Outlet />
          </>
        ) : activeChatId ? (
          <div className="flex h-full flex-col overflow-hidden">
            <div
              className={cn(
                'flex flex-shrink-0 items-center border-b border-border px-3 font-medium',
                IS_MAC ? 'h-[44px] text-[13px]' : 'h-[34px] text-xs',
              )}
              data-tauri-drag-region
            >
              {chatTabLabel}
            </div>
            <div className="flex min-h-0 flex-1 overflow-hidden bg-background">
              <Outlet />
            </div>
          </div>
        ) : (
          // overflow-visible (not hidden) so the content pane's drop shadow can
          // render past this wrapper toward the sidebar instead of being clipped
          // at the boundary. The pane clips its own content via its own overflow.
          <div className="flex h-full flex-col bg-transparent">
            <Outlet />
          </div>
        )}
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
