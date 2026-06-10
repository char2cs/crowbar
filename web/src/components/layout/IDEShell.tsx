import { useState, useRef, useEffect } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
import { SidebarProjectHeader } from './sidebar-project-header'
import { SidebarTabBar } from './sidebar-tab-bar'
import { SidebarCarousel } from './sidebar-carousel'
import { IS_MAC } from '@/utils/platform'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceView } from '@/features/workspace/components/WorkspaceView'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { Toaster } from '@/components/ui/sonner'
import { ConnectionIndicator } from './connection-indicator'

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
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const chats = useSidebarStore((s) => s.chats)
  const repos = useSidebarStore((s) => s.repos)
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const sidebarPanelRef = useRef<PanelImperativeHandle | null>(null)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]
  const activeRepo = repos.find((r) => r.workspaces?.some((ws) => ws.id === activeWorkspaceId))
  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'
  const chatTabLabel = chats.find((c) => c.id === activeChatId)?.title ?? 'Chat'

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
      <SidebarProjectHeader
        onProjectsClick={() => void navigate({ to: '/projects' })}
        onProjectSelect={() => void navigate({ to: '/' })}
      />
      <SidebarTabBar />
      <ErrorBoundary>
        <SidebarCarousel activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
      </ErrorBoundary>
    </div>
  )

  const contentEl = (
    <div className="flex h-full min-w-0 flex-col overflow-hidden bg-transparent">
      <ErrorBoundary>
        {activeWorkspaceId ? (
          <WorkspaceView wsId={activeWorkspaceId} />
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
      <Toaster />
    </SidebarProvider>
  )
}
