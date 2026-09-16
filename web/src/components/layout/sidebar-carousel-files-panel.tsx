import { useCallback, Suspense } from 'react'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resolveOnscreenPaneForWorkspace } from '@/features/panes/lib/pane-chat-workspace'
import { pickAndUploadFiles } from '@/features/files/lib/file-upload'

/**
 * The carousel's Files panel — every file-system-store binding and the
 * `FileExplorerTree` they drive, split out of `sidebar-carousel.tsx` because
 * this is its own whole store surface (selection, create/rename/delete/
 * upload) that none of the carousel's own tab/fold/resize machinery touches.
 */
export function SidebarCarouselFilesPanel({
  activeWorkspaceRepoPath,
}: {
  activeWorkspaceRepoPath: string
}) {
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
  // Live-reported: opening a file from the explorer could land it in a
  // DIFFERENT chat than the one the user was looking at, when that chat
  // shared its workspace ("group") with another one on screen. The explorer
  // click never named which pane it meant — see resolveOnscreenPaneForWorkspace's
  // own doc for the full mechanism. Reasserting the active pane here, right
  // before the open, is the same fix the file-tree DROP path already applies
  // for its own unambiguous drop target.
  const ensureActivePaneForFileOpen = useCallback(() => {
    const targetPaneId = resolveOnscreenPaneForWorkspace(getWorkspaceScope()?.wsId ?? '')
    if (targetPaneId) windowPaneStore.getState().paneActions.setActivePane(targetPaneId)
  }, [])

  return (
    <div
      data-testid="carousel-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      <ErrorBoundary>
        <Suspense fallback={<SidebarSkeleton />}>
          <FileExplorerTree
            files={files}
            rootFolderPath={activeWorkspaceRepoPath}
            onFileSelect={(path, isDir) => {
              if (isDir) {
                useFileTreeStore
                  .getState()
                  .toggleFolder(getWorkspaceScope()?.wsId ?? '', path)
              } else {
                ensureActivePaneForFileOpen()
                handleFileSelect?.(path, false)
              }
            }}
            onFileOpen={
              handleFileOpen
                ? (path: string, isDir: boolean) => {
                    if (!isDir) {
                      ensureActivePaneForFileOpen()
                      void handleFileOpen(path, false)
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
