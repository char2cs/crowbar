import { createRootRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar } from '@/components/layout/Sidebar'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat, deleteMockChat } from '@/lib/mock/chats'
import { deleteMockWorkspace } from '@/lib/mock/workspaces'

function RootLayout() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } = useSidebarStore()

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const handleNewChat = () => {
    const chat = createMockChat()
    addChat({ id: chat.id, title: chat.title, age: chat.age })
    navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  const handleDeleteChat = (id: string) => {
    deleteMockChat(id)
    deleteChat(id)
    if (activeChatId === id) {
      const remaining = chats.filter(c => c.id !== id)
      remaining.length > 0
        ? navigate({ to: '/chat/$chatId', params: { chatId: remaining[0].id } })
        : navigate({ to: '/' })
    }
  }

  const handleDeleteWorkspace = (wsId: string) => {
    deleteMockWorkspace(wsId)
    deleteWorkspace(wsId)
    if (activeWorkspaceId === wsId) navigate({ to: '/' })
  }

  return (
    <AppShell
      sidebar={
        <Sidebar
          userInitials="MU"
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={handleNewChat}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
          onDeleteChat={handleDeleteChat}
          onDeleteWorkspace={handleDeleteWorkspace}
          onRepoToggle={toggleRepo}
          onProjectsClick={() => navigate({ to: '/projects' })}
          onProjectSelect={() => navigate({ to: '/' })}
        />
      }
    >
      <ErrorBoundary>
        <Outlet />
      </ErrorBoundary>
    </AppShell>
  )
}

export const Route = createRootRoute({ component: RootLayout })
