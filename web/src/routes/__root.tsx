import { createRootRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar, type Repo, type ProjectChat } from '@/components/layout/Sidebar'

const MOCK_CHATS: ProjectChat[] = [
  { id: 'c1', title: 'Architecture decisions', age: '2h' },
  { id: 'c2', title: 'Auth strategy across services', age: '5d' },
]

const MOCK_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws3', num: 3, branch: 'feature/app-design', added: 5672, age: '16h ago' },
      { id: 'ws2', num: 2, branch: 'feature/api-backend', added: 27347, deleted: 455, age: '1d ago' },
      { id: 'ws1', num: 1, branch: 'enhancement/scaffold', added: 22892, age: '3d ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [{ id: 'qc1', branch: 'develop', age: '3d ago' }],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd1', branch: 'develop', age: '6d ago' },
      { id: 'qd2', branch: 'feature/quiver-shell', added: 13485, deleted: 69, age: '3d ago' },
    ],
  },
]

function RootLayout() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  return (
    <AppShell
      sidebar={
        <Sidebar
          projectName="Rabbyte"
          userInitials="MU"
          chats={MOCK_CHATS}
          repos={MOCK_REPOS}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={() => navigate({ to: '/chat/$chatId', params: { chatId: 'new' } })}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
        />
      }
    >
      <Outlet />
    </AppShell>
  )
}

export const Route = createRootRoute({ component: RootLayout })
