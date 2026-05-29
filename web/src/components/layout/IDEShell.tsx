import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider, Sidebar, SidebarInset } from '@/components/ui/sidebar'
import { SidebarHeader } from './SidebarHeader'
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
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)
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

  return (
    <SidebarProvider className="h-screen overflow-hidden bg-transparent text-foreground">
      <Sidebar side={sidebarPosition} collapsible="offcanvas">
        <div
          className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[44px]' : 'h-[34px]')}
          data-tauri-drag-region
        >
          <SidebarNavIcons />
        </div>
        <ErrorBoundary>
          <SidebarHeader
            userInitials="MU"
            onProjectsClick={() => void navigate({ to: '/projects' })}
            onProjectSelect={() => void navigate({ to: '/' })}
            onSettingsClick={() => setSettingsOpen(true)}
          />
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
      </Sidebar>

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

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <TerminalHost />
      <FontStyleInjector />
      <Toaster />
    </SidebarProvider>
  )
}
