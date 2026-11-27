import { useCallback, useState } from 'react'
import { findFileInTree } from '@/features/file-system/controllers/file-tree-utils'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { FileEntry } from '@/features/file-system/types/app'
import { getDirName, joinPath, stripTrailingPathSeparators } from '@/utils/path-helpers'
import { report } from '@/lib/errors/report'

interface UseFileExplorerInlineEditingProps {
  workspaceId: string | null
  files: FileEntry[]
  rootFolderPath?: string
  onUpdateFiles?: (files: FileEntry[]) => void
  onRenamePath?: (path: string, newName?: string) => void
  onCreateNewFileInDirectory: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onCreateNewFolderInDirectory?: (directoryPath: string, folderName: string) => void
  showAlertDialog: (title: string, message: string) => void
}

function clearEditingFlag(items: FileEntry[], path: string): FileEntry[] {
  return items.map((item) =>
    item.path === path
      ? { ...item, isRenaming: false, isEditing: false }
      : item.children
        ? { ...item, children: clearEditingFlag(item.children, path) }
        : item,
  )
}

function removeNewItems(items: FileEntry[]): FileEntry[] {
  const result: FileEntry[] = []
  for (const i of items) {
    if (i.isNewItem && i.isEditing) continue
    result.push({ ...i, children: i.children ? removeNewItems(i.children) : undefined })
  }
  return result
}

export function useFileExplorerInlineEditing({
  workspaceId,
  files,
  rootFolderPath,
  onUpdateFiles,
  onRenamePath,
  onCreateNewFileInDirectory,
  onCreateNewFolderInDirectory,
  showAlertDialog,
}: UseFileExplorerInlineEditingProps) {
  const [editingValue, setEditingValue] = useState('')

  const startInlineEditing = useCallback(
    (parentPath: string, isFolder: boolean) => {
      if (!onUpdateFiles) return

      const baseFiles = removeNewItems(files)

      const isRootTarget = !parentPath || parentPath === rootFolderPath

      const newItem: FileEntry = {
        name: '',
        path: isRootTarget ? '' : `${parentPath}/`,
        isDir: isFolder,
        isEditing: true,
        isNewItem: true,
      }

      const addNewItemToTree = (items: FileEntry[], targetPath: string): FileEntry[] =>
        items.map((item) => {
          if (item.path === targetPath && item.isDir) {
            return { ...item, children: [newItem, ...(item.children || [])] }
          }
          if (item.children) {
            return { ...item, children: addNewItemToTree(item.children, targetPath) }
          }
          return item
        })

      if (isRootTarget || parentPath === getDirName(baseFiles[0]?.path ?? '')) {
        onUpdateFiles([newItem, ...baseFiles])
      } else {
        onUpdateFiles(addNewItemToTree(baseFiles, parentPath))
      }

      if (parentPath) {
        const wsId = workspaceId ?? ''
        const { getExpandedPaths, setExpandedPaths } = useFileTreeStore.getState()
        setExpandedPaths(wsId, new Set(getExpandedPaths(wsId)).add(parentPath))
      }

      setEditingValue('')
    },
    [files, onUpdateFiles, rootFolderPath, workspaceId],
  )

  const finishInlineEditing = useCallback(
    (item: FileEntry, newName: string) => {
      if (!onUpdateFiles) return

      if (newName.trim()) {
        const parentPath = stripTrailingPathSeparators(item.path)

        if (item.isRenaming) {
          const trimmed = newName.trim()
          if (trimmed !== item.name) {
            Promise.resolve(onRenamePath?.(item.path, trimmed)).catch((error: unknown) => {
              report(error, { action: 'Rename failed' })
            })
          }
          onUpdateFiles(clearEditingFlag(files, item.path))
          setEditingValue('')
          return
        }

        if (item.isDir) {
          const folder = findFileInTree(files, joinPath(parentPath, newName.trim()))
          if (folder) {
            showAlertDialog('Folder Already Exists', 'A folder with this name already exists.')
            return
          }
          onCreateNewFolderInDirectory?.(parentPath, newName.trim())
        } else {
          const file = findFileInTree(files, joinPath(parentPath, newName.trim()))
          if (file) {
            showAlertDialog('File Already Exists', 'A file with this name already exists.')
            return
          }
          onCreateNewFileInDirectory(parentPath, newName.trim())
        }
      }

      onUpdateFiles(removeNewItems(files))
      setEditingValue('')
    },
    [
      files,
      onUpdateFiles,
      showAlertDialog,
      onRenamePath,
      onCreateNewFileInDirectory,
      onCreateNewFolderInDirectory,
    ],
  )

  const cancelInlineEditing = useCallback(
    (file: FileEntry) => {
      if (!onUpdateFiles) return

      if (file.isRenaming) {
        onUpdateFiles(clearEditingFlag(files, file.path))
        setEditingValue('')
        return
      }

      onUpdateFiles(removeNewItems(files))
      setEditingValue('')
    },
    [files, onUpdateFiles],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, file: FileEntry) => {
      if (e.key === 'Enter') {
        e.preventDefault()
        e.stopPropagation()
        finishInlineEditing(file, editingValue)
      } else if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        cancelInlineEditing(file)
      }
    },
    [editingValue, finishInlineEditing, cancelInlineEditing],
  )

  const handleBlur = useCallback(
    (file: FileEntry) => {
      if (editingValue.trim()) finishInlineEditing(file, editingValue)
      else cancelInlineEditing(file)
    },
    [editingValue, finishInlineEditing, cancelInlineEditing],
  )

  return {
    editingValue,
    setEditingValue,
    startInlineEditing,
    handleKeyDown,
    handleBlur,
  }
}
