import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
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

  const sidebarPanel = (
    <ResizablePanel id="sidebar" defaultSize="20%" minSize="12%" maxSize="45%" className="flex flex-col overflow-hidden">
      <div className="flex h-full flex-col overflow-hidden bg-chrome-bg backdrop-blur-sm">
        {/* Unified titlebar strip — no border here so both strips read as one bar */}
        <div
          className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[38px]' : 'h-[28px]')}
          data-tauri-drag-region
        >
          <SidebarNavIcons />
        </div>
        {/* Sidebar content — border starts here, below the unified strip */}
        <div className={cn(
          'flex flex-1 flex-col overflow-hidden',
          sidebarPosition === 'right' ? 'border-l border-border' : 'border-r border-border',
        )}>
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
        </div>
      </div>
    </ResizablePanel>
  )

  const contentPanel = (
    <ResizablePanel id="content" className="flex min-w-0 flex-1 flex-col overflow-hidden">
      {/* h-full wrapper gives children a definite height — no separate drag strip here;
          the tab bar is already h-[38px]/h-[28px] and has data-tauri-drag-region */}
      <div className="flex h-full flex-col overflow-hidden">
        <ErrorBoundary>
          {activeWorkspaceId ? (
            <WorkspaceView wsId={activeWorkspaceId} label={workspaceLabel} />
          ) : activeChatId ? (
            <div className="flex h-full flex-col overflow-hidden">
              {/* Chrome chat-title strip — platform-adaptive height matches the tab bar */}
              <div
                className={cn(
                  'flex flex-shrink-0 items-center border-b border-border bg-chrome-bg backdrop-blur-sm px-3 font-medium',
                  IS_MAC ? 'h-[38px] text-[13px]' : 'h-[28px] text-xs',
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
    </ResizablePanel>
  )

  return (
    <div className="flex h-screen overflow-hidden bg-transparent text-foreground">
      <ResizablePanelGroup orientation="horizontal" className="h-full">
        {sidebarPosition === "right" ? (
          <>{contentPanel}<ResizableHandle />{sidebarPanel}</>
        ) : (
          <>{sidebarPanel}<ResizableHandle />{contentPanel}</>
        )}
      </ResizablePanelGroup>

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <TerminalHost />
      <FontStyleInjector />
      <Toaster />
    </div>
  )
}
