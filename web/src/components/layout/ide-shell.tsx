import { useState, useRef, useEffect } from 'react'
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
import { useProjectStore } from '@/lib/store/projects'
import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { Toaster } from '@/components/ui/sonner'
import { ConnectionIndicator } from './connection-indicator'
import { FpsOverlay } from './fps-overlay'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

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
  const ideMatch = pathname.match(/\/ide\/([^/]+)\/([^/]+)\/([^/]+)/)
  const activeProjectIdFromRoute = ideMatch?.[1]
  const activeRepoIdFromRoute = ideMatch?.[2]
  const activeWorkspaceId = ideMatch?.[3]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]
  const activeRepo = repos.find((r) => r.id === activeRepoIdFromRoute)
  // TODO(workspace-paths): `/repos/<repoId>` is a synthetic mock-era root prefix.
  // Backend paths are workspace-relative, and this fiction already caused a 404
  // bug. It threads through rootFolderPath into 40+ files (sidebar-carousel →
  // file-explorer, path-helpers root checks, gitignore root rules), so removing
  // it is not a contained change — replace with workspace-relative roots in a
  // dedicated pass. Until then, key it off the route repo id.
  const activeWorkspaceRepoPath = activeRepoIdFromRoute
    ? `/repos/${activeRepoIdFromRoute}`
    : '/repos/default'
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
    <div className="flex h-full flex-col overflow-hidden bg-transparent select-none">
      {!hasNavScreen && <SidebarProjectHeader />}
      {!hasNavScreen && <ContextPill />}
      {!hasNavScreen && <SidebarTabBar />}
      <ErrorBoundary>
        <SidebarCarousel activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
      </ErrorBoundary>
    </div>
  )

  const contentEl = (
    <div className="flex h-full min-w-0 flex-col overflow-hidden bg-transparent">
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
          <div className="flex h-full flex-col overflow-hidden bg-background">
            <Outlet />
          </div>
        )}
      </ErrorBoundary>
    </div>
  )

  return (
    <SidebarProvider
      className="h-screen overflow-hidden bg-transparent text-foreground"
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
      <Toaster />
    </SidebarProvider>
  )
}
