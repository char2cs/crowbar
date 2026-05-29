import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider, Sidebar, SidebarInset, SidebarFooter } from '@/components/ui/sidebar'
import { SidebarProjectSwitcher } from './sidebar-project-switcher'
import { SidebarTabs } from './SidebarTabs'
import { SidebarNavIcons } from './sidebar-nav-icons'
import { IS_MAC } from '@/utils/platform'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { WorkspaceView } from '@/features/workspace/components/WorkspaceView'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { Toaster } from '@/components/ui/sonner'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const chats = useSidebarStore((s) => s.chats)
  const repos = useSidebarStore((s) => s.repos)
  const collapsedRepos = useSidebarStore((s) => s.collapsedRepos)
  const addChat = useSidebarStore((s) => s.addChat)
  const deleteChat = useSidebarStore((s) => s.deleteChat)
  const deleteWorkspace = useSidebarStore((s) => s.deleteWorkspace)
  const toggleRepo = useSidebarStore((s) => s.toggleRepo)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    try {
      const stored = parseInt(localStorage.getItem('sidebar-width') ?? '', 10)
      return Number.isFinite(stored) ? Math.max(294, stored) : 294
    } catch {
      return 294
    }
  })
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find(r => r.workspaces.some(ws => ws.id === activeWorkspaceId))
  const activeWorkspace = activeRepo?.workspaces.find(ws => ws.id === activeWorkspaceId)
  const workspaceLabel = activeWorkspace
    ? `${activeRepo?.name ?? ''} / ${activeWorkspace.branch}`
    : undefined

  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'
  const chatTabLabel = chats.find(c => c.id === activeChatId)?.title ?? 'Chat'

  function handleResizeDragStart(e: React.MouseEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = sidebarWidth
    const position = sidebarPosition

    // Target only the two elements that actually change width — bypasses CSS var cascade
    const gapEl = document.querySelector<HTMLElement>('[data-slot="sidebar-gap"]')
    const containerEl = document.querySelector<HTMLElement>('[data-slot="sidebar-container"]')

    // Disable transitions so they don't fight the rapid updates during drag
    if (gapEl) gapEl.style.transition = 'none'
    if (containerEl) containerEl.style.transition = 'none'

    let currentWidth = startWidth
    let latestX = startX
    let rafId: number | null = null

    function onMouseMove(e: MouseEvent) {
      latestX = e.clientX
      if (rafId !== null) return
      rafId = requestAnimationFrame(() => {
        const delta = position === 'left' ? latestX - startX : startX - latestX
        currentWidth = Math.min(640, Math.max(250, startWidth + delta))
        if (gapEl) gapEl.style.width = `${currentWidth}px`
        if (containerEl) containerEl.style.width = `${currentWidth}px`
        rafId = null
      })
    }

    function onMouseUp() {
      if (rafId !== null) cancelAnimationFrame(rafId)
      // Update CSS var to match, then clear inline overrides so toggle animation still works
      if (gapEl) { gapEl.style.transition = ''; gapEl.style.width = '' }
      if (containerEl) { containerEl.style.transition = ''; containerEl.style.width = '' }
      localStorage.setItem('sidebar-width', String(currentWidth))
      setSidebarWidth(currentWidth)
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
    }

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }

  const sidebarEl = (
    <Sidebar side={sidebarPosition} collapsible="offcanvas" className="[&>[data-slot=sidebar-inner]]:bg-transparent !border-0">
      <div className="relative flex h-full flex-col overflow-hidden">
        <div
          data-testid="sidebar-resize-handle"
          className={cn(
            'absolute inset-y-0 z-50 w-1 cursor-col-resize opacity-0 transition-opacity hover:opacity-100 hover:bg-border',
            sidebarPosition === 'right' ? 'left-0' : 'right-0',
          )}
          onMouseDown={handleResizeDragStart}
        />
        <div
          className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[44px]' : 'h-[34px]')}
          data-tauri-drag-region
        >
          <SidebarNavIcons />
        </div>
        <ErrorBoundary>
          <SidebarTabs
            chats={chats}
            repos={repos}
            collapsedRepos={collapsedRepos}
            activeChatId={activeChatId}
            activeWorkspaceId={activeWorkspaceId}
            activeWorkspaceRepoPath={activeWorkspaceRepoPath}
            onChatClick={id => void navigate({ to: '/chat/$chatId', params: { chatId: id } })}
            onWorkspaceClick={(_repoId, wsId) => void navigate({ to: '/workspaces/$wsId', params: { wsId } })}
            onNewChat={() => {
              const chat = createMockChat()
              addChat({ id: chat.id, title: chat.title, age: chat.age })
              void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
            }}
            onNewWorkspace={() => void navigate({ to: '/workspaces/new' })}
            onDeleteChat={id => { deleteChat(id); if (activeChatId === id) void navigate({ to: '/' }) }}
            onDeleteWorkspace={wsId => {
              deleteWorkspace(wsId)
              destroyWorkspaceStore(wsId)
              if (activeWorkspaceId === wsId) void navigate({ to: '/' })
            }}
            onRepoToggle={toggleRepo}
          />
        </ErrorBoundary>
        <SidebarFooter className="p-0">
          <SidebarProjectSwitcher
            onProjectsClick={() => void navigate({ to: '/projects' })}
            onProjectSelect={() => void navigate({ to: '/' })}
            onSettingsClick={() => setSettingsOpen(true)}
          />
        </SidebarFooter>
      </div>
    </Sidebar>
  )

  const contentEl = (
    <SidebarInset className="min-w-0 overflow-hidden bg-transparent">
      <div className="flex h-full flex-col overflow-hidden">
        <ErrorBoundary>
          {activeWorkspaceId ? (
            <WorkspaceView wsId={activeWorkspaceId} label={workspaceLabel} />
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
    </SidebarInset>
  )

  return (
    <SidebarProvider
      className="h-screen overflow-hidden bg-transparent text-foreground"
      style={{ '--sidebar-width': `${sidebarWidth}px` } as React.CSSProperties}
    >
      {sidebarPosition === 'right' ? <>{contentEl}{sidebarEl}</> : <>{sidebarEl}{contentEl}</>}
      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <TerminalHost />
      <FontStyleInjector />
      <Toaster />
    </SidebarProvider>
  )
}
