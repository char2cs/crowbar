import { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { MainSidebar } from '@/features/layout/components/sidebar/main-sidebar'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { SidebarTabs } from './SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { ErrorBoundary } from '@/components/ErrorBoundary'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { chats, repos, collapsedRepos, deleteChat, deleteWorkspace, toggleRepo } = useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find(r => r.workspaces.some(ws => ws.id === activeWorkspaceId))
  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <MainSidebar>
        <SidebarTabs
          userInitials="MU"
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          activeWorkspaceRepoPath={activeWorkspaceRepoPath}
          onChatClick={(id) => void navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_repoId, wsId) => void navigate({ to: '/workspaces/$wsId', params: { wsId } })}
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
      </MainSidebar>
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border">
        <ErrorBoundary>
          <SplitViewRoot />
        </ErrorBoundary>
      </div>
      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  )
}
