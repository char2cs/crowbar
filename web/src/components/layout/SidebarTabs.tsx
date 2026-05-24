import { useState, Suspense } from 'react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { WorkspacesSidebarPanel } from './WorkspacesSidebarPanel'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import GitView from '@/features/git/components/git-view'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import type { ProjectChat, Repo } from '@/lib/store/sidebar'

type SidebarTab = 'workspaces' | 'files' | 'git'

interface SidebarTabsProps {
  userInitials: string
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
  onProjectsClick?: () => void
  onProjectSelect?: (id: string) => void
  onSettingsOpen?: () => void
}

export function SidebarTabs({
  userInitials, chats, repos, collapsedRepos,
  activeChatId, activeWorkspaceId, activeWorkspaceRepoPath,
  onChatClick, onWorkspaceClick, onNewChat, onNewWorkspace,
  onDeleteChat, onDeleteWorkspace, onRepoToggle,
  onProjectsClick, onProjectSelect, onSettingsOpen,
}: SidebarTabsProps) {
  const [activeTab, setActiveTab] = useState<SidebarTab>('workspaces')

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsList className="h-9 w-full justify-start gap-0 rounded-none border-b border-border bg-card px-0">
        <TabsTrigger
          value="workspaces"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Workspaces
        </TabsTrigger>
        <TabsTrigger
          value="files"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Files
        </TabsTrigger>
        <TabsTrigger
          value="git"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Git
        </TabsTrigger>
      </TabsList>

      <TabsContent value="workspaces" forceMount hidden={activeTab !== 'workspaces'} className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspacesSidebarPanel
          userInitials={userInitials}
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
          onProjectsClick={onProjectsClick}
          onProjectSelect={onProjectSelect}
          onSettingsClick={onSettingsOpen}
        />
      </TabsContent>

      <TabsContent value="files" forceMount hidden={activeTab !== 'files'} className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={[]}
              rootFolderPath={activeWorkspaceRepoPath}
              onFileSelect={() => {}}
              onCreateNewFileInDirectory={() => {}}
            />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>

      <TabsContent value="git" forceMount hidden={activeTab !== 'git'} className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <GitView repoPath={activeWorkspaceRepoPath} />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>
    </Tabs>
  )
}
