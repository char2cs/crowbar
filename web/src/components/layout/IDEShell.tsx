import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
import { SidebarHeader } from './SidebarHeader'
import { SidebarTabs } from './SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { WorkspaceView } from '@/features/workspace/components/WorkspaceView'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'

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
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <ResizablePanelGroup orientation="horizontal" className={cn("h-full", sidebarPosition === "right" && "flex-row-reverse")}>

        <ResizablePanel defaultSize="20%" minSize="12%" maxSize="45%" className="flex flex-col overflow-hidden">
          <div className="flex h-full flex-col overflow-hidden border-r border-border bg-card">
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
                onDeleteWorkspace={wsId => { deleteWorkspace(wsId); if (activeWorkspaceId === wsId) void navigate({ to: '/' }) }}
                onRepoToggle={toggleRepo}
              />
            </ErrorBoundary>
          </div>
        </ResizablePanel>

        <ResizableHandle />

        <ResizablePanel className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <ErrorBoundary>
            {activeWorkspaceId ? (
              <WorkspaceView wsId={activeWorkspaceId} label={workspaceLabel} />
            ) : activeChatId ? (
              <div className="flex h-full flex-col overflow-hidden">
                <div className="flex items-center border-b border-border px-3 py-1 text-sm font-medium">
                  {chatTabLabel}
                </div>
                <Outlet />
              </div>
            ) : (
              <div className="flex h-full flex-col overflow-hidden">
                <Outlet />
              </div>
            )}
          </ErrorBoundary>
        </ResizablePanel>

      </ResizablePanelGroup>

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <TerminalHost />
    </div>
  )
}
