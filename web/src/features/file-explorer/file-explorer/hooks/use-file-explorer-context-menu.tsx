import {
  CaretDoubleUp,
  Copy,
  PencilSimple as Edit,
  FilePlus,
  FileText,
  FolderOpen,
  FolderPlus,
  Image as ImageIcon,
  Info,
  Link,
  ArrowClockwise as RefreshCw,
  Scissors,
  TerminalWindow as Terminal,
  Trash,
  Warning,
} from '@phosphor-icons/react'
import { useCallback, useMemo, useState } from 'react'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { readFile as readTextFile, writeFile } from '@/features/file-system/controllers/platform'
import {
  buildEnvTemplateContent,
  ENV_TEMPLATE_TARGETS,
  isEnvFileName,
} from '@/features/file-explorer/lib/env-template'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { ContextMenuState } from '@/features/file-system/types/app'
import { Button } from '@/components/ui/button'
import { ContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { AppDialog as Dialog } from '@/components/ui/dialog'
import { getBaseName, getDirName, getRelativePath, joinPath } from '@/utils/path-helpers'

interface UseFileExplorerContextMenuOptions {
  rootFolderPath?: string
  onFileSelect: (path: string, isDir: boolean) => void | Promise<void>
  onCreateNewFileInDirectory?: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onCreateNewFolderInDirectory?: (directoryPath: string, folderName: string) => void
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

interface EnvOverwriteDialogState {
  sourcePath: string
  targetFileName: string
}

interface PropertiesDialogState {
  fileName: string
  path: string
  size: string
  type: string
}

const menuIconSpacer = <span aria-hidden="true" />

function formatFileSize(sizeHeader: string | null): string {
  const bytes = Number(sizeHeader)
  if (!Number.isFinite(bytes) || bytes < 0) return 'Unknown'
  if (bytes < 1024) return `${bytes} bytes`

  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }

  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

export function useFileExplorerContextMenu({
  rootFolderPath,
  onFileSelect,
  onCreateNewFileInDirectory,
  onCreateNewFolderInDirectory,
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
  const [envOverwriteDialog, setEnvOverwriteDialog] = useState<EnvOverwriteDialogState | null>(null)
  const [propertiesDialog, setPropertiesDialog] = useState<PropertiesDialogState | null>(null)
  const [fileFeedback, setFileFeedback] = useState<
    Map<string, 'copied-path' | 'copied-rel' | 'created' | 'err'>
  >(new Map())
  const clipboardActions = useFileClipboardStore((state) => state.actions)
  const clipboard = useFileClipboardStore((state) => state.clipboard)

  function flashFeedback(path: string, kind: 'copied-path' | 'copied-rel' | 'created' | 'err') {
    setFileFeedback((prev) => new Map(prev).set(path, kind))
    setTimeout(() => {
      setFileFeedback((prev) => {
        const next = new Map(prev)
        next.delete(path)
        return next
      })
    }, 1500)
  }

  const createEnvTemplateFile = useCallback(
    async (sourcePath: string, targetFileName: string, options?: { overwrite?: boolean }) => {
      if (!onCreateNewFileInDirectory) return

      const directoryPath = getDirName(sourcePath)
      const targetPath = joinPath(directoryPath, targetFileName)

      try {
        if (targetPath === sourcePath) {
          // Silent no-op: same-name env file is an edge case; skip without feedback
          return
        }

        let targetExists = false
        try {
          await readTextFile(targetPath)
          targetExists = true
          if (!options?.overwrite) {
            setEnvOverwriteDialog({ sourcePath, targetFileName })
            return
          }
        } catch {
          /* intentionally ignored */
        }

        const sourceContent = await readTextFile(sourcePath)
        const templateContent = buildEnvTemplateContent(sourceContent)
        const createdPath = targetExists
          ? targetPath
          : (await Promise.resolve(onCreateNewFileInDirectory(directoryPath, targetFileName))) ||
            targetPath

        await writeFile(createdPath, templateContent)

        const wsStore = getActiveWorkspaceStoreRef()
        if (wsStore) {
          const wsState = wsStore.getState()
          const createdBuffer = wsState.buffers.find((buffer) => buffer.path === createdPath)
          if (createdBuffer) {
            wsStore.setState((state) => ({
              ...state,
              buffers: state.buffers.map((b) =>
                b.id === createdBuffer.id && 'content' in b
                  ? { ...b, content: templateContent }
                  : b,
              ),
            }))
          }
        }

        onRefreshDirectory?.(directoryPath)
        flashFeedback(createdPath, 'created')
      } catch (error) {
        console.error('Failed to create env template file:', error)
      }
    },
    [onCreateNewFileInDirectory, onRefreshDirectory],
  )

  const handleEnvOverwriteConfirm = useCallback(() => {
    if (!envOverwriteDialog) return
    const { sourcePath, targetFileName } = envOverwriteDialog
    setEnvOverwriteDialog(null)
    void createEnvTemplateFile(sourcePath, targetFileName, { overwrite: true })
  }, [createEnvTemplateFile, envOverwriteDialog])

  const handleContextMenu = useCallback((e: React.MouseEvent, filePath: string, isDir: boolean) => {
    e.preventDefault()
    e.stopPropagation()

    let x = e.pageX
    let y = e.pageY
    const menuWidth = 250
    const menuHeight = 400

    if (x + menuWidth > window.innerWidth) x = window.innerWidth - menuWidth
    if (y + menuHeight > window.innerHeight) y = window.innerHeight - menuHeight

    setContextMenu({ x, y, path: filePath, isDir })
  }, [])

  const contextMenuItems = useMemo<ContextMenuItem[]>(() => {
    if (!contextMenu) return []

    const items: ContextMenuItem[] = []

    if (contextMenu.isDir) {
      // Empty-space (root) right-click passes the absolute wsId as contextMenu.path,
      // but tree nodes are worktree-relative (root === ''). Normalise so the
      // directory actions address the worktree root, not a node literally named
      // the wsId. isWorkspaceRootPath is true for the wsId root (and any added
      // workspace root) and false for a real subdir like "api". Only the action
      // onClicks use this — NOT the Rename/Delete visibility gate below, which
      // must keep reading contextMenu.path so those stay hidden on the root.
      const isRootTarget = isWorkspaceRootPath?.(contextMenu.path) ?? false
      const dirTargetPath = isRootTarget ? '' : contextMenu.path

      items.push(
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
            if (isRootTarget) treeStore.collapseAll()
            else treeStore.collapsePath(contextMenu.path)
          },
        },
        {
          id: 'open-terminal',
          label: 'Open in Terminal',
          icon: <Terminal />,
          onClick: () => {
            const folderName = getBaseName(dirTargetPath, 'terminal')
            getActiveWorkspaceStoreRef()?.getState().bufferActions.openContent({
              type: 'terminal',
              name: folderName,
              workingDirectory: dirTargetPath,
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

      items.push({ id: 'sep-dir', label: '', separator: true, onClick: () => {} })
    } else {
      const fileName = getBaseName(contextMenu.path, '')
      const canCreateEnvTemplate =
        isEnvFileName(fileName) &&
        !contextMenu.path.startsWith('remote://') &&
        Boolean(onCreateNewFileInDirectory)

      items.push(
        {
          id: 'open',
          label: 'Open',
          icon: <FolderOpen />,
          onClick: () => onFileSelect(contextMenu.path, false),
        },
        {
          id: 'copy-content',
          label: 'Copy Content',
          icon: <Copy />,
          onClick: async () => {
            try {
              const response = await fetch(contextMenu.path)
              const content = await response.text()
              await navigator.clipboard.writeText(content)
            } catch {
              /* intentionally ignored */
            }
          },
        },
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
          onClick: async () => {
            const fileName = getBaseName(contextMenu.path, '')
            const extension = fileName.includes('.') ? fileName.split('.').pop() : undefined
            let size = 'Unknown'

            try {
              const stats = await fetch(`file://${contextMenu.path}`, { method: 'HEAD' })
              size = formatFileSize(stats.headers.get('content-length'))
            } catch {
              /* intentionally ignored */
            }

            setPropertiesDialog({
              fileName,
              path: contextMenu.path,
              size,
              type: extension || 'No extension',
            })
          },
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
          // contextMenu.path is root-relative for tree nodes (e.g. "api/main.go").
          // Join with rootFolderPath to produce the absolute path, unless the path
          // is already absolute (happens when right-clicking the workspace root itself,
          // which passes rootFolderPath directly as contextMenu.path).
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
      {
        id: 'copy',
        label: 'Copy',
        icon: <Copy />,
        onClick: () => {
          clipboardActions.copy([{ path: contextMenu.path, is_dir: contextMenu.isDir }])
        },
      },
      {
        id: 'cut',
        label: 'Cut',
        icon: <Scissors />,
        onClick: () => {
          clipboardActions.cut([{ path: contextMenu.path, is_dir: contextMenu.isDir }])
        },
      },
    )

    if (shouldShowFileManagementItems) {
      items.push(
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
    clipboard,
    clipboardActions,
    contextMenu,
    createEnvTemplateFile,
    onCreateNewFolderInDirectory,
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
  ])

  const hasDialog = Boolean(envOverwriteDialog || propertiesDialog)
  const contextMenuElement =
    contextMenu || hasDialog ? (
      <>
        {contextMenu && (
          <ContextMenu
            isOpen
            position={{ x: contextMenu.x ?? 0, y: contextMenu.y ?? 0 }}
            items={contextMenuItems}
            onClose={() => setContextMenu(null)}
            className="file-tree-context-menu min-w-[220px]"
          />
        )}

        {envOverwriteDialog && (
          <Dialog
            title="Overwrite Env File"
            icon={Warning}
            onClose={() => setEnvOverwriteDialog(null)}
            size="sm"
            footer={
              <>
                <Button variant="ghost" onClick={() => setEnvOverwriteDialog(null)}>
                  Cancel
                </Button>
                <Button variant="destructive" onClick={handleEnvOverwriteConfirm} compact>
                  Overwrite
                </Button>
              </>
            }
          >
            <p className="ui-font ui-text-sm text-foreground">
              {envOverwriteDialog.targetFileName} already exists. Overwrite it?
            </p>
          </Dialog>
        )}

        {propertiesDialog && (
          <Dialog
            title="Properties"
            icon={Info}
            onClose={() => setPropertiesDialog(null)}
            size="md"
          >
            <dl className="grid grid-cols-[72px_1fr] gap-x-3 gap-y-2 ui-font ui-text-sm">
              <dt className="text-muted-foreground">File</dt>
              <dd className="min-w-0 break-words text-foreground">{propertiesDialog.fileName}</dd>
              <dt className="text-muted-foreground">Path</dt>
              <dd className="min-w-0 break-words text-foreground">{propertiesDialog.path}</dd>
              <dt className="text-muted-foreground">Size</dt>
              <dd className="text-foreground">{propertiesDialog.size}</dd>
              <dt className="text-muted-foreground">Type</dt>
              <dd className="text-foreground">{propertiesDialog.type}</dd>
            </dl>
          </Dialog>
        )}
      </>
    ) : null

  return {
    contextMenu,
    setContextMenu,
    handleContextMenu,
    contextMenuElement,
    fileFeedback,
  }
}
