import { useCallback, useState } from 'react'
import { findFileInTree } from '@/features/file-system/controllers/file-tree-utils'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { FileEntry } from '@/features/file-system/types/app'
import { getDirName, joinPath, stripTrailingPathSeparators } from '@/utils/path-helpers'

interface UseFileExplorerInlineEditingProps {
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

// Clear the inline-edit flags on the node at `path`. Rename START is an
// idempotent SET in the store; cancel and commit must clear the flag here so the
// editable state is never left stuck on a node — a stuck isRenaming made the next
// double-click toggle the input OFF, i.e. "rename sometimes does nothing".
function clearEditingFlag(items: FileEntry[], path: string): FileEntry[] {
  return items.map((item) =>
    item.path === path
      ? { ...item, isRenaming: false, isEditing: false }
      : item.children
        ? { ...item, children: clearEditingFlag(item.children, path) }
        : item,
  )
}

// Strip every in-progress "new item" placeholder from the tree. A New File/Folder
// dismissed without committing can leave its placeholder behind (the "already
// exists" early-return in finishInlineEditing, or a spurious blur on appear), and
// a second placeholder shares the same empty root path — colliding on React keys,
// which silently breaks rendering so the next inline input never appears (the
// "can't create files until reload" bug). startInlineEditing clears these first.
function removeNewItems(items: FileEntry[]): FileEntry[] {
  return items
    .filter((i) => !(i.isNewItem && i.isEditing))
    .map((i) => ({
      ...i,
      children: i.children ? removeNewItems(i.children) : undefined,
    }))
}

export function useFileExplorerInlineEditing({
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

      // The workspace root is addressed by its absolute path (=== rootFolderPath)
      // even though the tree's own nodes use root-relative paths (root === '').
      // Normalise that to the empty-string root so the new item lands at the top
      // level and finishInlineEditing derives an empty parent directory.
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

      // Expand the parent so the new inline-edit row is visible — but NEVER for
      // the root (''): the root is always shown, and adding '' to the expanded
      // set makes the new path:'' node look like an expanded directory, which
      // then loads the workspace root's contents as its own children and renders
      // the entire tree a second time (the "folder name + whole tree" popup bug).
      if (parentPath) {
        try {
          const current = useFileTreeStore.getState().getExpandedPaths()
          const next = new Set(current)
          next.add(parentPath)
          useFileTreeStore.getState().setExpandedPaths(next)
        } catch {
          /* intentionally ignored */
        }
      }

      setEditingValue('')
    },
    [files, onUpdateFiles, rootFolderPath],
  )

  const finishInlineEditing = useCallback(
    (item: FileEntry, newName: string) => {
      if (!onUpdateFiles) return

      if (newName.trim()) {
        // An empty parentPath is the (valid) workspace root: tree paths are
        // root-relative, so the create handlers join the name onto '' to write
        // at the worktree root. Do NOT substitute rootFolderPath here — that is
        // the absolute wsId path and would create at wsId/<name> instead.
        const parentPath = stripTrailingPathSeparators(item.path)

        if (item.isRenaming) {
          const trimmed = newName.trim()
          // Only hit the rename API if the name actually changed (a blur with the
          // pre-filled name should not round-trip a no-op rename).
          if (trimmed !== item.name) onRenamePath?.(item.path, trimmed)
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
