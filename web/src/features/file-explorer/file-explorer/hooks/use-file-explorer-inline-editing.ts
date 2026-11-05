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

// Clear the inline-edit flags on the node at `path`. Starting a rename only sets the flag, so
// cancel and commit must clear it: a stuck isRenaming makes the next double-click close the input.
function clearEditingFlag(items: FileEntry[], path: string): FileEntry[] {
  return items.map((item) =>
    item.path === path
      ? { ...item, isRenaming: false, isEditing: false }
      : item.children
        ? { ...item, children: clearEditingFlag(item.children, path) }
        : item,
  )
}

// Strip every uncommitted "new item" placeholder (one can survive a dismissal). Two share the
// empty root path and collide on React keys, which stops the next inline input from rendering,
// so startInlineEditing clears them first.
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

      // Clear any stale placeholder from a prior edit that was dismissed without
      // committing, so a second New File/Folder never collides with it (see
      // removeNewItems). Operate on the cleaned tree throughout.
      const baseFiles = removeNewItems(files)

      // The root target arrives as its absolute path (=== rootFolderPath) but tree nodes are
      // root-relative, so normalise it to '' and finishInlineEditing derives an empty parent.
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

      // Expand the parent so the new row shows; never the root (''), whose
      // expansion would load the root's contents again as its own children.
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
        // '' is the valid workspace root: create handlers join the name onto it. Never substitute
        // rootFolderPath, the absolute wsId path, which would create at wsId/<name>.
        const parentPath = stripTrailingPathSeparators(item.path)

        if (item.isRenaming) {
          const trimmed = newName.trim()
          // Rename only when the name changed (a blur with the pre-filled name is a no-op).
          // onRenamePath is fire-and-forget, so a rejection (locked, name collision …) is toasted.
          if (trimmed !== item.name) {
            Promise.resolve(onRenamePath?.(item.path, trimmed)).catch((error: unknown) => {
              report(error, { action: 'Rename failed' })
            })
          }
          // Clear the flag now so the node is immediately non-editing; the WS
          // refetch will later replace it with a fresh node at the new path. Do
          // not depend on that refetch to clear the editable state.
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
        // Clear the editable flag directly (do NOT call bare onRenamePath, which
        // now START-s a rename) so Escape deterministically closes the input.
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
