import { useCallback, Suspense } from 'react'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { resolveOnscreenPaneForWorkspace } from '@/features/panes/lib/pane-chat-workspace'
import { pickAndUploadFiles } from '@/features/files/lib/file-upload'

/**
 * The carousel's Files panel — every file-system-store binding and the
 * `FileExplorerTree` they drive, split out of `sidebar-carousel.tsx` because
 * this is its own whole store surface (selection, create/rename/delete/
 * upload) that none of the carousel's own tab/fold/resize machinery touches.
 */
export function SidebarCarouselFilesPanel() {
  const workspaceId = useFocusedWorkspaceContextStore((s) => s.workspaceId)
  const rootPath = useFocusedWorkspaceContextStore((s) => s.rootPath)
  const files = useFileSystemStore((s) => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()
  // File-tree mutation handlers (create/rename/delete/refresh) live on the
  // file-system store; thread them into the explorer so its context menu and
  // inline-edit actions actually run (the daemon backs them via /files).
  const setFiles = useFileSystemStore((s) => s.setFiles)
  const handleCreateNewFileInDirectory = useFileSystemStore.use.handleCreateNewFileInDirectory?.()
  const handleCreateNewFolderInDirectory =
    useFileSystemStore.use.handleCreateNewFolderInDirectory?.()
  const handleRenamePath = useFileSystemStore.use.handleRenamePath?.()
  const handleDeletePath = useFileSystemStore.use.handleDeletePath?.()
  const handleDuplicatePath = useFileSystemStore.use.handleDuplicatePath?.()
  const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.()
  const refreshDirectory = useFileSystemStore.use.refreshDirectory?.()
  const handleUploadFile = useCallback(
    (directoryPath: string) => void pickAndUploadFiles(directoryPath),
    [],
  )
  // The explorer click names the pane it opens into (C8): the on-screen pane
  // of this workspace — never merely whichever pane had focus, which can be a
  // different chat sharing the workspace (see resolveOnscreenPaneForWorkspace).
  const fileOpenTarget = useCallback(
    () => ({ paneId: resolveOnscreenPaneForWorkspace(workspaceId ?? '') ?? undefined }),
    [workspaceId],
  )

  return (
    <div
      data-testid="carousel-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      <ErrorBoundary>
        <Suspense fallback={<SidebarSkeleton />}>
          <FileExplorerTree
            files={files}
            workspaceId={workspaceId}
            rootFolderPath={rootPath}
            onFileSelect={(path, isDir) => {
              if (isDir) {
                useFileTreeStore.getState().toggleFolder(workspaceId ?? '', path)
              } else {
                handleFileSelect?.(path, false, fileOpenTarget())
              }
            }}
            onFileOpen={
              handleFileOpen
                ? (path: string, isDir: boolean) => {
                    if (!isDir) {
                      void handleFileOpen(path, false, fileOpenTarget())
                    }
                  }
                : undefined
            }
            onUpdateFiles={setFiles}
            onCreateNewFileInDirectory={handleCreateNewFileInDirectory ?? (() => {})}
            onCreateNewFolderInDirectory={handleCreateNewFolderInDirectory ?? undefined}
            onRenamePath={handleRenamePath ?? undefined}
            onDeletePath={handleDeletePath ?? undefined}
            onDuplicatePath={handleDuplicatePath ?? undefined}
            onRevealInFinder={handleRevealInFolder ?? undefined}
            onUploadFile={handleUploadFile}
            onRefreshDirectory={refreshDirectory ?? undefined}
          />
        </Suspense>
      </ErrorBoundary>
    </div>
  )
}
