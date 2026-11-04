import {
  CaretDoubleUp,
  Copy,
  CopySimple,
  PencilSimple as Edit,
  FilePlus,
  FileText,
  FolderMinus,
  FolderOpen,
  FolderPlus,
  FolderSimplePlus,
  Image as ImageIcon,
  Info,
  Link,
  ArrowClockwise as RefreshCw,
  Scissors,
  TerminalWindow as Terminal,
  Trash,
  UploadSimple,
} from '@phosphor-icons/react'
import { useCallback, useMemo, useState } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { readDocument, workspaceRef } from '@/lib/api/documents'
import { report } from '@/lib/errors/report'
import { ENV_TEMPLATE_TARGETS, isEnvFileName } from '@/features/file-explorer/lib/env-template'
import { FilePropertiesDialog } from '@/features/file-explorer/file-explorer/components/file-properties-dialog'
import { useEnvTemplateFile } from '@/features/file-explorer/file-explorer/hooks/use-env-template-file'
import {
  fileProperties,
  type FileProperties,
} from '@/features/file-explorer/file-explorer/lib/file-properties'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { ContextMenuState } from '@/features/file-system/types/app'
import { ContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { getBaseName, getRelativePath, joinPath } from '@/utils/path-helpers'

interface UseFileExplorerContextMenuOptions {
  /** The workspace the tree shows — every workspace-keyed action uses it. */
  workspaceId: string | null
  rootFolderPath?: string
  /** A locked worktree refuses every write, so mutation items are hidden; read-only ones stay. */
  isLocked?: boolean
  onFileSelect: (path: string, isDir: boolean) => void | Promise<void>
  onCreateNewFileInDirectory?: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onGenerateImage?: (directoryPath: string) => void
  onRefreshDirectory?: (path: string) => void
  onRenamePath?: (path: string, newName?: string) => void
  onRevealInFinder?: (path: string) => void
  onUploadFile?: (directoryPath: string) => void
  onDuplicatePath?: (path: string) => void
  onAddFolderToWorkspace?: () => void
  onRemoveFolderFromWorkspace?: (path: string) => void
  isWorkspaceRootPath?: (path: string) => boolean
  canRemoveWorkspaceRootPath?: (path: string) => boolean
  onDeleteRequested: (candidate: { path: string; isDir: boolean }) => void
  onStartInlineEditing: (path: string, isFolder: boolean) => void
  onOpenAllFilesInDirectory: (directoryPath: string) => Promise<void>
}

const menuIconSpacer = <span aria-hidden="true" />

async function copyFileContent(workspaceId: string, path: string): Promise<void> {
  try {
    const { text } = await readDocument(workspaceRef(workspaceId, path))
    await navigator.clipboard.writeText(text)
  } catch (err) {
    report(err, { action: 'Copy the file content' })
  }
}

export function useFileExplorerContextMenu({
  workspaceId,
  rootFolderPath,
  isLocked = false,
  onFileSelect,
  onCreateNewFileInDirectory,
  onGenerateImage,
  onRefreshDirectory,
  onRenamePath,
  onRevealInFinder,
  onUploadFile,
  onDuplicatePath,
  onAddFolderToWorkspace,
  onRemoveFolderFromWorkspace,
  isWorkspaceRootPath,
  canRemoveWorkspaceRootPath,
  onDeleteRequested,
  onStartInlineEditing,
  onOpenAllFilesInDirectory,
}: UseFileExplorerContextMenuOptions) {
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [propertiesDialog, setPropertiesDialog] = useState<FileProperties | null>(null)
  const [fileFeedback, setFileFeedback] = useState<
    Map<string, 'copied-path' | 'copied-rel' | 'created' | 'err'>
  >(new Map())
  const clipboardActions = useFileClipboardStore((state) => state.actions)

  const flashFeedback = useCallback(
    (path: string, kind: 'copied-path' | 'copied-rel' | 'created' | 'err') => {
      setFileFeedback((prev) => new Map(prev).set(path, kind))
      setTimeout(() => {
        setFileFeedback((prev) => {
          const next = new Map(prev)
          next.delete(path)
          return next
        })
      }, 1500)
    },
    [],
  )
  const onEnvFileCreated = useCallback(
    (path: string) => flashFeedback(path, 'created'),
    [flashFeedback],
  )
  const { createEnvTemplateFile, envOverwriteDialog } = useEnvTemplateFile({
    workspaceId,
    onCreateNewFileInDirectory,
    onRefreshDirectory,
    onCreated: onEnvFileCreated,
  })

  const handleContextMenu = useCallback((e: React.MouseEvent, filePath: string, isDir: boolean) => {
    e.preventDefault()
    e.stopPropagation()

    let x = e.pageX
    let y = e.pageY
    const menuWidth = 250
    const menuHeight = 400

    if (x + menuWidth > window.innerWidth) x = window.innerWidth - menuWidth
    if (y + menuHeight > window.innerHeight) y = window.innerHeight - menuHeight

    setContextMenu({ x, y, path: filePath, isDir, isOpen: true })
  }, [])

  // Closing keeps the menu mounted until base-ui finishes closing it, so an
  // item that moved focus (an inline name input) keeps it.
  const closeContextMenu = useCallback(() => {
    setContextMenu((menu) => menu && { ...menu, isOpen: false })
  }, [])

  const contextMenuItems = useMemo<ContextMenuItem[]>(() => {
    if (!contextMenu) return []

    const items: ContextMenuItem[] = []
    const staged = [{ path: contextMenu.path, is_dir: contextMenu.isDir }]

    if (contextMenu.isDir) {
      // A root right-click passes the root itself as the path while tree nodes are
      // worktree-relative (root === ''): the actions address ''. The Rename/Delete
      // gate below keeps reading contextMenu.path, so those stay hidden on the root.
      const isRootTarget = isWorkspaceRootPath?.(contextMenu.path) ?? false
      const dirTargetPath = isRootTarget ? '' : contextMenu.path

      items.push(
        // New File / New Folder / Upload mutate the worktree — hidden on a locked
        // workspace (every write there is a guaranteed 409). Refresh / Open All /
        // Collapse All / Open in Terminal below are read-only and always shown.
        ...(isLocked
          ? []
          : [
              {
                id: 'new-file',
                label: 'New File',
                icon: <FilePlus />,
                onClick: () => onStartInlineEditing(dirTargetPath, false),
              },
              {
                id: 'new-folder',
                label: 'New Folder',
                icon: <FolderPlus />,
                onClick: () => onStartInlineEditing(dirTargetPath, true),
              },
            ]),
        ...(onUploadFile && workspaceId && !isLocked
          ? [
              {
                id: 'upload-file',
                label: 'Upload File',
                icon: <UploadSimple />,
                onClick: () => onUploadFile(dirTargetPath),
              },
            ]
          : []),
        {
          id: 'refresh',
          label: 'Refresh',
          icon: <RefreshCw />,
          onClick: () => {
            void onRefreshDirectory?.(dirTargetPath)
          },
        },
        {
          id: 'open-all-files',
          label: 'Open All Files',
          icon: <FolderOpen />,
          onClick: () => void onOpenAllFilesInDirectory(dirTargetPath),
        },
        {
          id: 'collapse-all',
          label: 'Collapse All',
          icon: <CaretDoubleUp />,
          onClick: () => {
            // '' doesn't match relative paths, so the root collapses everything
            // via collapseAll(); a subdir collapses just its own subtree.
            const treeStore = useFileTreeStore.getState()
            const wsId = workspaceId ?? ''
            if (isRootTarget) treeStore.collapseAll(wsId)
            else treeStore.collapsePath(wsId, contextMenu.path)
          },
        },
        {
          id: 'open-terminal',
          label: 'Open in Terminal',
          icon: <Terminal />,
          onClick: () => {
            if (!workspaceId) return
            const folderName = getBaseName(dirTargetPath, 'terminal')
            windowPaneStore.getState().bufferActions.openContent({
              type: 'terminal',
              name: folderName,
              workingDirectory: dirTargetPath,
              workspaceId,
            })
          },
        },
      )

      if (onGenerateImage) {
        items.push({
          id: 'generate-image',
          label: 'Generate Image',
          icon: <ImageIcon />,
          onClick: () => onGenerateImage(contextMenu.path),
        })
      }

      // "Add Folder to Workspace" is a workspace-level affordance: its impl takes
      // no path (it opens a folder picker), so it belongs on the root target only,
      // not on every subdirectory.
      if (isRootTarget && onAddFolderToWorkspace) {
        items.push({
          id: 'add-folder-to-workspace',
          label: 'Add Folder to Workspace',
          icon: <FolderSimplePlus />,
          onClick: () => onAddFolderToWorkspace(),
        })
      }

      items.push({ id: 'sep-dir', label: '', separator: true, onClick: () => {} })
    } else {
      const fileName = getBaseName(contextMenu.path, '')
      // Env-template items create a sibling .env file — a mutation, so hidden on
      // a locked workspace and without one.
      const canCreateEnvTemplate =
        Boolean(workspaceId && onCreateNewFileInDirectory) && !isLocked && isEnvFileName(fileName)

      items.push(
        {
          id: 'open',
          label: 'Open',
          icon: <FolderOpen />,
          onClick: () => onFileSelect(contextMenu.path, false),
        },
        ...(workspaceId
          ? [
              {
                id: 'copy-content',
                label: 'Copy Content',
                icon: <Copy />,
                onClick: () => void copyFileContent(workspaceId, contextMenu.path),
              },
            ]
          : []),
        ...(canCreateEnvTemplate
          ? [
              { id: 'sep-env-template', label: '', separator: true, onClick: () => {} },
              ...ENV_TEMPLATE_TARGETS.map((target, index) => ({
                id: target.id,
                label: target.label,
                icon: index === 0 ? <FilePlus /> : menuIconSpacer,
                onClick: () => void createEnvTemplateFile(contextMenu.path, target.fileName),
              })),
            ]
          : []),
        {
          id: 'properties',
          label: 'Properties',
          icon: <Info />,
          onClick: () => setPropertiesDialog(fileProperties(contextMenu.path)),
        },
        { id: 'sep-file', label: '', separator: true, onClick: () => {} },
      )
    }

    const shouldShowFileManagementItems =
      !contextMenu.isDir || !isWorkspaceRootPath?.(contextMenu.path)

    items.push(
      {
        id: 'copy-path',
        label: 'Copy Path',
        icon: <Link />,
        onClick: () => {
          // Tree nodes are root-relative; the workspace root itself arrives absolute.
          const absolutePath =
            rootFolderPath && !contextMenu.path.startsWith(rootFolderPath)
              ? joinPath(rootFolderPath, contextMenu.path)
              : contextMenu.path
          navigator.clipboard.writeText(absolutePath).then(
            () => flashFeedback(contextMenu.path, 'copied-path'),
            () => flashFeedback(contextMenu.path, 'err'),
          )
        },
      },
      {
        id: 'copy-relative-path',
        label: 'Copy Relative Path',
        icon: <FileText />,
        onClick: () => {
          // getRelativePath strips rootFolderPath when the path is absolute.
          // For already-relative paths it returns the path unchanged, which is correct.
          const relativePath = getRelativePath(contextMenu.path, rootFolderPath)
          navigator.clipboard.writeText(relativePath || '.').then(
            () => flashFeedback(contextMenu.path, 'copied-rel'),
            () => flashFeedback(contextMenu.path, 'err'),
          )
        },
      },
      ...(onRevealInFinder
        ? [
            {
              id: 'reveal',
              label: 'Reveal in Finder',
              icon: <FolderOpen />,
              onClick: () => {
                // Tree nodes carry a root-relative path; Finder needs the absolute
                // one. Mirror the Copy Path join (skip when already absolute, which
                // happens for the workspace root itself).
                const absolutePath =
                  rootFolderPath && !contextMenu.path.startsWith(rootFolderPath)
                    ? joinPath(rootFolderPath, contextMenu.path)
                    : contextMenu.path
                onRevealInFinder(absolutePath)
              },
            },
          ]
        : []),
      {
        id: 'copy',
        label: 'Copy',
        icon: <Copy />,
        onClick: () => void clipboardActions.copy(staged, workspaceId),
      },
      // Cut stages a move whose completion (paste) deletes from this workspace —
      // a mutation, so hidden when locked. Copy above only reads, so it stays.
      ...(isLocked
        ? []
        : [
            {
              id: 'cut',
              label: 'Cut',
              icon: <Scissors />,
              onClick: () => void clipboardActions.cut(staged, workspaceId),
            },
          ]),
      ...(onRemoveFolderFromWorkspace && canRemoveWorkspaceRootPath?.(contextMenu.path)
        ? [
            {
              id: 'remove-from-workspace',
              label: 'Remove From Workspace',
              icon: <FolderMinus />,
              onClick: () => onRemoveFolderFromWorkspace(contextMenu.path),
            },
          ]
        : []),
    )

    // The file-management group (Duplicate / Rename / Delete) mutates the
    // worktree — hidden entirely on a locked workspace.
    if (shouldShowFileManagementItems && !isLocked) {
      items.push(
        ...(onDuplicatePath
          ? [
              {
                id: 'duplicate',
                label: 'Duplicate',
                icon: <CopySimple />,
                onClick: () => onDuplicatePath(contextMenu.path),
              },
            ]
          : []),
        {
          id: 'rename',
          label: 'Rename',
          icon: <Edit />,
          onClick: () => onRenamePath?.(contextMenu.path),
        },
        { id: 'sep-end', label: '', separator: true, onClick: () => {} },
        {
          id: 'delete',
          label: 'Delete',
          icon: <Trash />,
          className: 'text-red-400',
          onClick: () => onDeleteRequested({ path: contextMenu.path, isDir: contextMenu.isDir }),
        },
      )
    }

    return items
  }, [
    canRemoveWorkspaceRootPath,
    clipboardActions,
    contextMenu,
    createEnvTemplateFile,
    flashFeedback,
    isLocked,
    onCreateNewFileInDirectory,
    onDeleteRequested,
    onDuplicatePath,
    onFileSelect,
    onGenerateImage,
    onOpenAllFilesInDirectory,
    onAddFolderToWorkspace,
    onRemoveFolderFromWorkspace,
    onRefreshDirectory,
    onRenamePath,
    onRevealInFinder,
    onStartInlineEditing,
    onUploadFile,
    isWorkspaceRootPath,
    rootFolderPath,
    workspaceId,
  ])

  const hasDialog = Boolean(envOverwriteDialog || propertiesDialog)
  const contextMenuElement =
    contextMenu || hasDialog ? (
      <>
        {contextMenu && (
          <ContextMenu
            isOpen={contextMenu.isOpen === true}
            position={{ x: contextMenu.x ?? 0, y: contextMenu.y ?? 0 }}
            items={contextMenuItems}
            onClose={closeContextMenu}
            className="file-tree-context-menu min-w-[220px]"
          />
        )}

        {envOverwriteDialog}
        {propertiesDialog && (
          <FilePropertiesDialog
            properties={propertiesDialog}
            onClose={() => setPropertiesDialog(null)}
          />
        )}
      </>
    ) : null

  return {
    contextMenu,
    closeContextMenu,
    handleContextMenu,
    contextMenuElement,
    fileFeedback,
  }
}
