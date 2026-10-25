import { useCallback, useMemo, Suspense } from 'react'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import {
  useRegisteredWorkspaceStore,
  useWorkspaceStoreById,
} from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { isFileTreeLoading } from '@/features/workspace/stores/slices/file-tree-slice'
import { createFileTreeHandlers, revealInFolder } from '@/features/files/lib/file-tree-handlers'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { resolveOnscreenPaneForWorkspace } from '@/features/panes/lib/pane-chat-workspace'
import { pickAndUploadFiles } from '@/features/files/lib/file-upload'
import { useWorkspaceIsLocal } from '@/lib/host-resolve'

/**
 * The sidebar's Files panel — every file-system-store binding and the
 * `FileExplorerTree` they drive (selection, create/rename/delete/upload).
 */
export function SidebarFilesPanel() {
  const workspaceId = useFocusedWorkspaceContextStore((s) => s.workspaceId)
  const rootPath = useFocusedWorkspaceContextStore((s) => s.rootPath)
  // The focused workspace's own tree: switching focus selects another store's
  // slice, it never clears or refetches anything.
  const wsId = workspaceId ?? ''
  const isLocal = useWorkspaceIsLocal(wsId)
  const store = useRegisteredWorkspaceStore(wsId)
  const files = useWorkspaceStoreById(wsId, (s) => s.files)
  const isLoading = useWorkspaceStoreById(wsId, isFileTreeLoading)
  const setFiles = useWorkspaceStoreById(wsId, (s) => s.fileTreeActions.setFiles)
  const handlers = useMemo(() => createFileTreeHandlers(wsId, store), [wsId, store])
  const handleUploadFile = useCallback(
    (directoryPath: string) => {
      if (workspaceId) void pickAndUploadFiles(workspaceId, directoryPath)
    },
    [workspaceId],
  )
  // The explorer click names the pane it opens into (C8): the on-screen pane
  // of this workspace — never merely whichever pane had focus, which can be a
  // different chat sharing the workspace (see resolveOnscreenPaneForWorkspace).
  const fileOpenTarget = useCallback(
    () => ({ paneId: resolveOnscreenPaneForWorkspace(workspaceId ?? '') ?? undefined }),
    [workspaceId],
  )

  return (
    <ErrorBoundary>
      <Suspense fallback={<SidebarSkeleton />}>
        <FileExplorerTree
          files={files}
          isLoading={isLoading}
          workspaceId={workspaceId}
          rootFolderPath={rootPath}
          onFileSelect={(path, isDir) => {
            if (isDir) {
              useFileTreeStore.getState().toggleFolder(workspaceId ?? '', path)
            } else {
              handlers.handleFileSelect(path, false, fileOpenTarget())
            }
          }}
          onFileOpen={(path: string, isDir: boolean) => {
            if (!isDir) {
              void handlers.handleFileOpen(path, false, fileOpenTarget())
            }
          }}
          onUpdateFiles={setFiles}
          onCreateNewFileInDirectory={handlers.handleCreateNewFileInDirectory}
          onCreateNewFolderInDirectory={handlers.handleCreateNewFolderInDirectory}
          onRenamePath={handlers.handleRenamePath}
          onDeletePath={handlers.handleDeletePath}
          onDuplicatePath={handlers.handleDuplicatePath}
          onRevealInFinder={isLocal ? (path) => revealInFolder(wsId, path) : undefined}
          onUploadFile={handleUploadFile}
          onRefreshDirectory={handlers.refreshDirectory}
        />
      </Suspense>
    </ErrorBoundary>
  )
}
