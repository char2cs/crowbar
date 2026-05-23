import { createRootRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useState } from 'react'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar, type Repo } from '@/components/layout/Sidebar'
import { getAllMockChats, createMockChat, deleteMockChat } from '@/lib/mock/chats'
import { deleteMockWorkspace } from '@/lib/mock/workspaces'

const INITIAL_REPOS: Repo[] = [
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
  const [chats, setChats] = useState(() =>
    getAllMockChats().map(c => ({ id: c.id, title: c.title, age: c.age })),
  )
  const [repos, setRepos] = useState(INITIAL_REPOS)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const handleNewChat = () => {
    const chat = createMockChat()
    setChats(prev => [...prev, { id: chat.id, title: chat.title, age: chat.age }])
    navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  const handleDeleteChat = (id: string) => {
    deleteMockChat(id)
    setChats(prev => {
      const next = prev.filter(c => c.id !== id)
      if (activeChatId === id) {
        if (next.length > 0) {
          navigate({ to: '/chat/$chatId', params: { chatId: next[0].id } })
        } else {
          navigate({ to: '/' })
        }
      }
      return next
    })
  }

  const handleDeleteWorkspace = (wsId: string) => {
    deleteMockWorkspace(wsId)
    setRepos(prev => prev.map(r => ({ ...r, workspaces: r.workspaces.filter(w => w.id !== wsId) })))
    if (activeWorkspaceId === wsId) {
      navigate({ to: '/' })
    }
  }

  return (
    <AppShell
      sidebar={
        <Sidebar
          projectName="Rabbyte"
          userInitials="MU"
          chats={chats}
          repos={repos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={handleNewChat}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
          onDeleteChat={handleDeleteChat}
          onDeleteWorkspace={handleDeleteWorkspace}
        />
      }
    >
      <Outlet />
    </AppShell>
  )
}

export const Route = createRootRoute({ component: RootLayout })
