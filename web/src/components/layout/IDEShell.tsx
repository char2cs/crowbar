import { useState, useEffect } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
import { SidebarHeader } from './SidebarHeader'
import { SidebarTabs } from './SidebarTabs'
import { IDETabBar } from './IDETabBar'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferStore } from '@/features/editor/stores/buffer-store'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import type { AppFile } from '@/features/file-system/types/app'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find((r) =>
    r.workspaces.some((ws) => ws.id === activeWorkspaceId),
  )
  const activeWorkspace = activeRepo?.workspaces.find((ws) => ws.id === activeWorkspaceId)
  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'
  const activeChat = chats.find((c) => c.id === activeChatId)

  const chatTabLabel = activeChat?.title ?? 'Chat'

  // Pane tree is rendered by SplitViewRoot below

  // Seed the mock file system store whenever the active workspace repo changes
  useEffect(() => {
    const files = getMockFileTree(activeWorkspaceRepoPath) as AppFile[]
    useFileSystemStore.setState({
      rootFolderPath: activeWorkspaceRepoPath,
      files,
      handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
        if (revealOrIsDir === true) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        useBufferStore.getState().actions.openContent({ type: 'editor', path, name, content })
      },
      handleFileSelect: (path: string, isDir?: boolean) => {
        if (isDir) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        useBufferStore.getState().actions.openContent({
          type: 'editor', path, name, content, isPreview: true,
        })
      },
    })
  }, [activeWorkspaceRepoPath])

  // Open the workspace chat as a tab in the PaneContainer whenever the active workspace changes
  useEffect(() => {
    if (!activeWorkspaceId) return
    const name = activeWorkspace
      ? `${activeRepo?.name ?? ''} / ${activeWorkspace.branch}`
      : 'Workspace'
    useBufferStore.getState().actions.openContent({
      type: 'crowbarChat',
      wsId: activeWorkspaceId,
      name,
    })
  }, [activeWorkspaceId]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <ResizablePanelGroup orientation="horizontal" className="h-full">

        {/* ── Sidebar ─────────────────────────────────────────── */}
        <ResizablePanel
          defaultSize="20%"
          minSize="12%"
          maxSize="45%"
          className="flex flex-col overflow-hidden"
        >
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
                onChatClick={(id) =>
                  void navigate({ to: '/chat/$chatId', params: { chatId: id } })
                }
                onWorkspaceClick={(_repoId, wsId) =>
                  void navigate({ to: '/workspaces/$wsId', params: { wsId } })
                }
                onNewChat={() => {
                  const chat = createMockChat()
                  addChat({ id: chat.id, title: chat.title, age: chat.age })
                  void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
                }}
                onNewWorkspace={() => void navigate({ to: '/workspaces/new' })}
                onDeleteChat={(id) => {
                  deleteChat(id)
                  if (activeChatId === id) void navigate({ to: '/' })
                }}
                onDeleteWorkspace={(wsId) => {
                  deleteWorkspace(wsId)
                  if (activeWorkspaceId === wsId) void navigate({ to: '/' })
                }}
                onRepoToggle={toggleRepo}
              />
            </ErrorBoundary>
          </div>
        </ResizablePanel>

        <ResizableHandle />

        {/* ── Main content ─────────────────────────────────────── */}
        <ResizablePanel className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <ErrorBoundary>
            {activeWorkspaceId ? (
              <SplitViewRoot />
            ) : activeChatId ? (
              <div className="flex h-full flex-col overflow-hidden">
                <IDETabBar
                  label={chatTabLabel}
                  onClose={() => void navigate({ to: '/' })}
                />
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
    </div>
  )
}
