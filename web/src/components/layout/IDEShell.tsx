import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
import { SidebarTabs } from './SidebarTabs'
import { FlowTab } from './FlowTab'
import { useSidebarStore } from '@/lib/store/sidebar'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { ErrorBoundary } from '@/components/ErrorBoundary'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { chats, repos, collapsedRepos, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find((r) =>
    r.workspaces.some((ws) => ws.id === activeWorkspaceId),
  )
  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <ResizablePanelGroup orientation="horizontal" className="h-full">
        {/* Sidebar */}
        <ResizablePanel
          defaultSize="20%"
          minSize="12%"
          maxSize="45%"
          className="flex flex-col overflow-hidden"
        >
          <div className="flex h-full flex-col overflow-hidden border-r border-border bg-card">
            <ErrorBoundary>
              <SidebarTabs
                userInitials="MU"
                chats={chats}
                repos={repos}
                collapsedRepos={collapsedRepos}
                activeChatId={activeChatId}
                activeWorkspaceId={activeWorkspaceId}
                activeWorkspaceRepoPath={activeWorkspaceRepoPath}
                onChatClick={(id) =>
                  void navigate({ to: '/chat/$chatId', params: { chatId: id } })
                }
                onWorkspaceClick={(_repoId, wsId) =>
                  void navigate({ to: '/workspaces/$wsId', params: { wsId } })
                }
                onNewChat={() => {}}
                onNewWorkspace={() => void navigate({ to: '/workspaces/new' })}
                onDeleteChat={(id) => deleteChat(id)}
                onDeleteWorkspace={(wsId) => {
                  deleteWorkspace(wsId)
                  if (activeWorkspaceId === wsId) void navigate({ to: '/' })
                }}
                onRepoToggle={toggleRepo}
                onProjectsClick={() => void navigate({ to: '/projects' })}
                onProjectSelect={() => void navigate({ to: '/' })}
                onSettingsOpen={() => setSettingsOpen(true)}
              />
            </ErrorBoundary>
          </div>
        </ResizablePanel>

        <ResizableHandle />

        {/* Main content area */}
        <ResizablePanel className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <ErrorBoundary>
            {activeWorkspaceId ? (
              <FlowTab workspaceId={activeWorkspaceId} />
            ) : (
              <div className="flex h-full flex-col overflow-hidden">
                <Outlet />
              </div>
            )}
          </ErrorBoundary>
        </ResizablePanel>
      </ResizablePanelGroup>

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  )
}
