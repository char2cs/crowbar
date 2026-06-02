import { Suspense } from 'react'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitHistoryList } from './git-history-list'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { SidebarSkeleton } from '@/components/layout/SidebarSkeleton'

export function GitPanel() {
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()

  return (
    <Tabs defaultValue="changes" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center px-2 py-1.5">
        <TabsList variant="default" className="w-full">
          <TabsTab value="changes" className="flex-1 justify-center">Changes</TabsTab>
          <TabsTab value="history" className="flex-1 justify-center">History</TabsTab>
        </TabsList>
      </div>

      <TabsPanel value="changes" className="flex flex-1 flex-col overflow-hidden">
        <Suspense fallback={<SidebarSkeleton />}>
          <FileExplorerTree
            files={files}
            filter="changed"
            onFileSelect={(path, isDir) => {
              if (isDir) useFileTreeStore.getState().toggleFolder(path)
            }}
            onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
              if (!isDir) void handleFileOpen(path, false)
            } : undefined}
            onCreateNewFileInDirectory={() => {}}
          />
        </Suspense>
      </TabsPanel>

      <TabsPanel value="history" className="flex flex-1 flex-col overflow-hidden">
        <GitHistoryList />
      </TabsPanel>
    </Tabs>
  )
}
