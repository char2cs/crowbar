import type React from 'react'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useEventListener } from '@/hooks/use-event-listener'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { fileOpenBenchmark } from '@/features/editor/utils/file-open-benchmark'
import {
  useFileSystemStore,
  workspaceFoldersSupported,
} from '@/features/file-system/controllers/store'
import type { FileEntry } from '@/features/file-system/types/app'
import { useSidebarStore } from '@/lib/store/sidebar'
import { isWorkspaceLockedInSidebar } from '@/lib/store/repo-tree'
import { SidebarEmptyActionState } from '@/components/ui/sidebar'
import { cn } from '@/utils/cn'
import { useFileExplorerContextMenu } from '../hooks/use-file-explorer-context-menu'
import { useFileExplorerDialogs } from '../hooks/use-file-explorer-dialogs'
import { useFileExplorerDragDrop } from '../hooks/use-file-explorer-drag-drop'
import { useFileExplorerInlineEditing } from '../hooks/use-file-explorer-inline-editing'
import { useFileExplorerSync } from '../hooks/use-file-explorer-sync'
import { useFileExplorerVisibleRows } from '../hooks/use-file-explorer-visible-rows'
import { useFilteredFileTree } from '../hooks/use-filtered-file-tree'
import { useTreeContainerEvents } from '../hooks/use-tree-container-events'
import { useTreeSearch, useTreeSearchNavigation } from '../hooks/use-tree-search'
import { FileExplorerDialogs } from './file-explorer-dialogs'
import { FileExplorerSearchHeader } from './file-explorer-search-header'
import { FileExplorerTreeRows, getFileTreeRowId } from './file-explorer-tree-rows'
import '../styles/file-explorer-tree.css'

interface FileExplorerTreeProps {
  /** The workspace this tree shows; keys every file-tree-store and daemon call. */
  workspaceId: string | null
  files: FileEntry[]
  activePath?: string
  updateActivePath?: (path: string) => void
  rootFolderPath?: string
  onFileSelect: (path: string, isDir: boolean) => void | Promise<void>
  onFileOpen?: (path: string, isDir: boolean) => void | Promise<void>
  onCreateNewFileInDirectory: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onCreateNewFolderInDirectory?: (directoryPath: string, folderName: string) => void
  onDeletePath?: (path: string, isDir: boolean) => void
  onGenerateImage?: (directoryPath: string) => void
  onUpdateFiles?: (files: FileEntry[]) => void
  onRenamePath?: (path: string, newName?: string) => void
  onDuplicatePath?: (path: string) => void
  onRefreshDirectory?: (path: string) => void
  onRevealInFinder?: (path: string) => void
  onUploadFile?: (directoryPath: string) => void
  onFileMove?: (oldPath: string, newPath: string) => void
  filter?: 'all' | 'changed'
}

const handleRootDrop = (e: React.DragEvent) => {
  e.preventDefault()
  e.stopPropagation()
}

/** Keep only changed files (and directories that contain one or are changed themselves). */
function pruneToChanged(
  items: FileEntry[],
  decorationOf: (file: FileEntry) => unknown,
): FileEntry[] {
  return items.flatMap((item) => {
    if (!item.isDir) return decorationOf(item) !== null ? [item] : []
    const children = pruneToChanged(item.children ?? [], decorationOf)
    if (children.length === 0 && decorationOf(item) === null) return []
    return [{ ...item, children }]
  })
}

function FileExplorerTreeComponent({
  workspaceId,
  files,
  activePath,
  updateActivePath,
  rootFolderPath,
  onFileSelect,
  onFileOpen,
  onCreateNewFileInDirectory,
  onCreateNewFolderInDirectory,
  onDeletePath,
  onGenerateImage,
  onUpdateFiles,
  onRenamePath,
  onDuplicatePath,
  onRefreshDirectory,
  onRevealInFinder,
  onUploadFile,
  onFileMove,
  filter = 'all' as const,
}: FileExplorerTreeProps) {
  const [focusedPath, setFocusedPath] = useState<string | undefined>(activePath)
  const [hasTreeFocus, setHasTreeFocus] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const documentRef = useRef<Document>(document)

  const handleOpenFolder = useFileSystemStore((state) => state.handleOpenFolder)
  const addFolderToWorkspace = useFileSystemStore((state) => state.addFolderToWorkspace)
  const removeFolderFromWorkspace = useFileSystemStore((state) => state.removeFolderFromWorkspace)
  const revealPathInTree = useFileSystemStore((state) => state.revealPathInTree)
  const isFileTreeLoading = useFileSystemStore((state) => state.isFileTreeLoading)

  // A locked (protected-branch) worktree refuses every daemon write (409
  // "workspace locked"), so the context menu must not offer mutations there.
  // isWorkspaceLockedInSidebar checks tree rows AND the default (main-worktree)
  // workspace. Returns a boolean, so the selector stays referentially stable.
  const isWorkspaceLocked = useSidebarStore((s) => isWorkspaceLockedInSidebar(s.repos, workspaceId))

  const handleAutoExpandDirectory = useCallback(
    (path: string) => {
      if (useFileTreeStore.getState().isExpanded(workspaceId ?? '', path)) return
      void Promise.resolve(onFileSelect(path, true))
    },
    [onFileSelect, workspaceId],
  )

  const { filteredFiles, workspaceRootPaths, isVisible, getGitStatusDecoration } =
    useFilteredFileTree({ workspaceId, files, rootFolderPath })

  const { showAlertDialog, requestDelete, openAllFilesInDirectory, dialogProps } =
    useFileExplorerDialogs({
      filteredFiles,
      isVisible,
      rootFolderPath,
      onFileSelect,
      onFileOpen,
      onDeletePath,
      updateActivePath,
    })

  const handleMoveError = useCallback(
    (message: string) => showAlertDialog('Move Failed', message),
    [showAlertDialog],
  )

  const { dragState, startDrag } = useFileExplorerDragDrop(
    rootFolderPath,
    onFileMove,
    handleAutoExpandDirectory,
    handleMoveError,
  )

  const search = useTreeSearch({ workspaceId, filteredFiles, containerRef })
  const { displayedFiles, setOpen: setSearchOpen } = search

  const treeFiles = useMemo(
    () =>
      filter === 'changed'
        ? pruneToChanged(displayedFiles, getGitStatusDecoration)
        : displayedFiles,
    [filter, displayedFiles, getGitStatusDecoration],
  )

  const { visibleRows, rowVirtualizer } = useFileExplorerVisibleRows({
    wsId: workspaceId ?? '',
    files: treeFiles,
    activePath,
    containerRef,
    expandedPathsOverride: search.displayedExpandedPaths,
  })

  const searchNavigation = useTreeSearchNavigation({
    search,
    visibleRows,
    rowVirtualizer,
    keyboardPath: focusedPath || activePath,
    setFocusedPath,
  })
  const keyboardPath = searchNavigation.cursorPath
  const highlightedPath = hasTreeFocus ? keyboardPath : activePath

  useEffect(() => {
    if (!hasTreeFocus) {
      // react-doctor-disable-next-line no-chain-state-updates -- FP: this is a one-way sync of the keyboard cursor to `activePath` while the tree is unfocused (so re-focus resumes at the active file). Nothing chains off `focusedPath` via another effect — it's only read at render (`keyboardPath = focusedPath || activePath`) and written by keyboard handlers — so there is no multi-step render cascade.
      setFocusedPath(activePath)
    }
  }, [activePath, hasTreeFocus])

  const { editingValue, setEditingValue, startInlineEditing, handleKeyDown, handleBlur } =
    useFileExplorerInlineEditing({
      workspaceId,
      files,
      rootFolderPath,
      onUpdateFiles,
      onRenamePath,
      onCreateNewFileInDirectory,
      onCreateNewFolderInDirectory,
      showAlertDialog,
    })

  const { setContextMenu, handleContextMenu, contextMenuElement, fileFeedback } =
    useFileExplorerContextMenu({
      workspaceId,
      rootFolderPath,
      // Locked worktrees refuse writes — hide every mutation item (New File/Folder,
      // Upload, Duplicate, Rename, Delete, Cut, env-template).
      isLocked: isWorkspaceLocked,
      onFileSelect,
      onCreateNewFileInDirectory,
      onGenerateImage,
      onRefreshDirectory,
      onRenamePath,
      onRevealInFinder,
      onUploadFile,
      onDuplicatePath,
      // The store impls behind these are still no-op stubs (no multi-root
      // workspace model exists yet); an undefined handler hides the menu items
      // so no dead actions render. See workspaceFoldersSupported (Task 28).
      onAddFolderToWorkspace: workspaceFoldersSupported
        ? () => {
            void addFolderToWorkspace()
          }
        : undefined,
      onRemoveFolderFromWorkspace: workspaceFoldersSupported
        ? (path) => {
            void removeFolderFromWorkspace(path)
          }
        : undefined,
      // Only the actual workspace root hides Rename/Delete. workspaceRootPaths
      // lists every top-level folder (a multi-root-workspace notion that doesn't
      // apply to Crowbar's single worktree); using it here wrongly treated every
      // top-level folder as a root, so Delete/Rename never appeared on them.
      isWorkspaceRootPath: (path) => path === rootFolderPath,
      canRemoveWorkspaceRootPath: (path) =>
        path !== rootFolderPath && workspaceRootPaths.includes(path),
      onDeleteRequested: requestDelete,
      onStartInlineEditing: startInlineEditing,
      onOpenAllFilesInDirectory: openAllFilesInDirectory,
    })

  useEventListener(
    'keydown',
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContextMenu(null)
    },
    documentRef,
  )

  useEventListener('dragover', (e: DragEvent) => e.preventDefault(), documentRef)

  const openSearch = useCallback(() => setSearchOpen(true), [setSearchOpen])
  const closeContextMenu = useCallback(() => setContextMenu(null), [setContextMenu])
  const containerEvents = useTreeContainerEvents({
    workspaceId,
    rootFolderPath,
    containerRef,
    visibleRows,
    rowVirtualizer,
    keyboardPath,
    setFocusedPath,
    updateActivePath,
    onFileSelect,
    onFileOpen,
    onRenamePath,
    onRefreshDirectory,
    openSearch,
    handleContextMenu,
    closeContextMenu,
    isDragging: dragState.isDragging,
    startDrag,
  })

  useEffect(() => {
    if (!activePath || !fileOpenBenchmark.has(activePath)) return
    fileOpenBenchmark.mark(activePath, 'explorer-active-path')
    const rafId = requestAnimationFrame(() => {
      fileOpenBenchmark.mark(activePath, 'explorer-painted')
    })
    return () => cancelAnimationFrame(rafId)
  }, [activePath])

  return (
    <div
      className={cn(
        // `px-1.5` matches the sidebar's own row gutter (`ROW_BASE`'s
        // `mx-1.5`) — a tree row has no margin of its own to create it, so
        // the container supplies the same 6px inset on both edges instead.
        'file-tree-container relative flex min-w-full flex-1 select-none flex-col overflow-auto px-1.5',
        dragState.dragOverPath === '__ROOT__' &&
          'border-2! border-dashed! border-secondary! bg-secondary! bg-opacity-10!',
      )}
      ref={containerRef}
      style={{ scrollBehavior: 'auto', overscrollBehavior: 'contain' }}
      role="tree"
      aria-label="File Explorer"
      aria-activedescendant={highlightedPath ? getFileTreeRowId(highlightedPath) : undefined}
      tabIndex={0}
      onFocusCapture={() => {
        setHasTreeFocus(true)
        setFocusedPath((current) => current || activePath || visibleRows[0]?.file.path)
      }}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setHasTreeFocus(false)
        }
      }}
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = dragState.draggedItem ? 'move' : 'copy'
      }}
      onDrop={handleRootDrop}
      {...containerEvents}
    >
      <FileExplorerSync
        activePath={activePath}
        updateActivePath={updateActivePath}
        revealPathInTree={revealPathInTree}
      />
      {search.isOpen && (
        <FileExplorerSearchHeader
          search={search}
          onNavigateMatch={searchNavigation.navigate}
          onClose={searchNavigation.closeSearch}
        />
      )}
      {!rootFolderPath ? (
        <div className="file-tree-empty-state flex flex-1 items-center justify-center">
          <SidebarEmptyActionState
            message="No folder open"
            actionLabel="Open Folder"
            onAction={handleOpenFolder}
          />
        </div>
      ) : displayedFiles.length === 0 ? (
        <div className="file-tree-empty-state flex flex-1 items-center justify-center">
          <SidebarEmptyActionState
            message={
              search.isSearching
                ? 'Searching files'
                : search.isActive
                  ? 'No matching files'
                  : isFileTreeLoading
                    ? 'Loading files…'
                    : 'Folder is empty'
            }
          />
        </div>
      ) : (
        // Horizontal gutter comes from the container's own `px-1.5` alone
        // (matching the sidebar row's `mx-1.5`) — a second `px-*` here would
        // double it, so only vertical breathing room stays local.
        <div id="file-tree-results" className="file-tree-scroll-body py-1">
          <FileExplorerTreeRows
            visibleRows={visibleRows}
            rowVirtualizer={rowVirtualizer}
            activePath={activePath}
            highlightedPath={highlightedPath}
            searchBarOpen={search.isOpen}
            searchQuery={search.isActive ? search.query : undefined}
            matchedPaths={search.result.matchedPaths}
            item={{
              dragOverPath: dragState.dragOverPath,
              isDragging: dragState.isDragging,
              editingValue,
              onEditingValueChange: setEditingValue,
              onKeyDown: handleKeyDown,
              onBlur: handleBlur,
              getGitStatusDecoration,
              fileFeedback,
            }}
          />
        </div>
      )}

      {contextMenuElement}
      <FileExplorerDialogs {...dialogProps} />
    </div>
  )
}

/**
 * `useFileExplorerSync` as a LEAF, not as a call in the tree's own body.
 *
 * That hook returns nothing — it is two `useEffect`s that point the explorer at
 * whatever file the active pane is showing — but it subscribes to the WINDOW's
 * `activeEditorTabId`, which moves every time focus crosses between a pane
 * holding an editor tab and one that doesn't (clicking between tiled chats in a
 * single workspace view does exactly that). Called inside
 * `FileExplorerTreeComponent`, each of those clicks re-rendered this whole
 * virtualized tree — ~365 fibers, every visible row, its git decorations and
 * its dropdowns — to produce identical output, because the thing that changed
 * was never rendered here in the first place. It is the same defect
 * `NavigationHistoryRecorder` (ide-shell.tsx) isolates one level up, and it was
 * hidden underneath it until that one was fixed: the tree sits inside
 * `IDEShell`, so it was being re-rendered from above anyway.
 *
 * Rendering it as a childless leaf keeps both effects, their timing and their
 * props exactly as they were while confining the re-render to one fiber. It
 * emits no DOM, so sitting inside the `role="tree"` container changes neither
 * layout nor the accessibility tree.
 */
function FileExplorerSync(props: {
  activePath?: string
  updateActivePath?: (path: string) => void
  revealPathInTree: (path: string) => void | Promise<void>
}): null {
  useFileExplorerSync(props)
  return null
}

export const FileExplorerTree = memo(FileExplorerTreeComponent)
export default FileExplorerTree
