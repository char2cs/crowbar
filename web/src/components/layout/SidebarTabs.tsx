import { Suspense } from 'react'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { WorkspacesSidebarPanel } from './WorkspacesSidebarPanel'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import GitView from '@/features/git/components/git-view'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { getMockFileTree } from '@/lib/mock/files'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useSidebarStore, type SidebarTab, type ProjectChat, type Repo } from '@/lib/store/sidebar'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

interface SidebarTabsProps {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos?: Set<string>
  activeChatId?: string
  activeWorkspaceId?: string
  activeWorkspaceRepoPath: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
  onDeleteChat?: (id: string) => void
  onDeleteWorkspace?: (wsId: string) => void
  onRepoToggle?: (repoId: string) => void
}

export function SidebarTabs({
  chats, repos, collapsedRepos,
  activeChatId, activeWorkspaceId, activeWorkspaceRepoPath,
  onChatClick, onWorkspaceClick, onNewChat, onNewWorkspace,
  onDeleteChat, onDeleteWorkspace, onRepoToggle,
}: SidebarTabsProps) {
  // Use persistent store so tab survives any re-mounts of this component.
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsContent value="workspaces" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspacesSidebarPanel
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={onChatClick}
          onWorkspaceClick={onWorkspaceClick}
          onNewChat={onNewChat}
          onNewWorkspace={onNewWorkspace}
          onDeleteChat={onDeleteChat}
          onDeleteWorkspace={onDeleteWorkspace}
          onRepoToggle={onRepoToggle}
        />
      </TabsContent>

      <TabsContent value="files" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={getMockFileTree(activeWorkspaceRepoPath)}
              rootFolderPath={activeWorkspaceRepoPath}
              onFileSelect={(path, isDir) => {
                if (isDir) {
                  useFileTreeStore.getState().toggleFolder(path)
                } else {
                  handleFileSelect?.(path, false)
                }
              }}
              onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
                if (!isDir) void handleFileOpen(path, false)
              } : undefined}
              onCreateNewFileInDirectory={() => {}}
            />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>

      <TabsContent value="git" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <GitView repoPath={activeWorkspaceRepoPath} />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>
    </Tabs>
  )
}
