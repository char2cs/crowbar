import { Suspense } from 'react'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { WorkspaceTree } from './workspace-tree'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

interface SidebarTabsProps {
  activeWorkspaceRepoPath: string
}

export function SidebarTabs({ activeWorkspaceRepoPath }: SidebarTabsProps) {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsContent value="workspaces" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspaceTree />
      </TabsContent>

      <TabsContent value="files" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={files}
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


    </Tabs>
  )
}
