import {
  copyFileNode,
  createFileNode,
  deleteFileNode,
  fetchFileTree,
  findNode,
  mergeChildren,
  renameFileNode,
} from '@/features/files/lib/file-tree-api'
import { openDocument } from '@/features/editor/documents/open-document'
import { workspaceRef } from '@/lib/api/documents'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AppFile } from '@/features/file-system/types/app'
import { revealItemInFinder } from '@/lib/crowbar-bridge'
import { workspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { report } from '@/lib/errors/report'
import { isNotFoundError } from '@/lib/errors/api-error'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { joinPath } from '@/utils/path-helpers'

export interface FileOpenTarget {
  paneId?: string
}

function parentDir(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

function duplicateDestPath(srcPath: string, files: AppFile[]): string {
  const dir = parentDir(srcPath)
  const name = dir ? srcPath.slice(dir.length + 1) : srcPath
  const dotIdx = name.lastIndexOf('.')
  const hasExt = dotIdx > 0
  const stem = hasExt ? name.slice(0, dotIdx) : name
  const ext = hasExt ? name.slice(dotIdx) : ''
  const taken = (candidate: string) =>
    findNode(files, dir ? joinPath(dir, candidate) : candidate) !== null

  let candidate = `${stem} copy${ext}`
  let n = 2
  while (taken(candidate)) {
    candidate = `${stem} copy ${n}${ext}`
    n += 1
  }
  return dir ? joinPath(dir, candidate) : candidate
}

function preserveLoadedChildren(current: AppFile[], fresh: AppFile[]): AppFile[] {
  return fresh.map((node) => {
    if (!node.isDir) return node
    const existing = findNode(current, node.path)
    if (existing?.children) return { ...node, children: existing.children }
    return node
  })
}

export async function refreshDirectory(
  wsId: string,
  store: WorkspaceStore,
  path?: string,
): Promise<void> {
  if (path && !findNode(store.getState().files, path)?.children) return
  let fresh: AppFile[]
  try {
    fresh = await fetchFileTree(wsId, path || undefined)
  } catch (err) {
    if (path && isNotFoundError(err)) useFileTreeStore.getState().collapsePath(wsId, path)
    else report(err, { action: 'Refresh the file tree' })
    return
  }
  const { files, fileTreeActions } = store.getState()
  const reconciled = preserveLoadedChildren(files, fresh)
  fileTreeActions.setFiles(!path ? reconciled : mergeChildren(files, path, reconciled))
}

export function revealInFolder(wsId: string, path: string): void {
  if (path.includes('://')) return
  const root = path.startsWith('/') ? '' : workspaceRootPath(wsId)
  if (root === undefined) return
  const absolute = root ? joinPath(root, path) : path
  revealItemInFinder(absolute).catch((error: unknown) => {
    report(error, { action: 'Reveal in Finder failed' })
  })
}

export function createFileTreeHandlers(wsId: string, store: WorkspaceStore) {
  const relativeDir = (dirPath: string) => (dirPath === wsId ? '' : dirPath)
  return {
    handleFileOpen: async (path: string, isDir?: boolean, opts?: FileOpenTarget) => {
      if (isDir) return
      await openDocument(workspaceRef(wsId, path), { paneId: opts?.paneId })
    },
    handleFileSelect: (path: string, isDir?: boolean, opts?: FileOpenTarget) => {
      if (isDir) return
      void openDocument(workspaceRef(wsId, path), { preview: true, paneId: opts?.paneId })
    },
    handleCreateNewFileInDirectory: async (dirPath: string, fileName?: string) => {
      if (!fileName) return
      const dir = relativeDir(dirPath)
      const path = dir ? joinPath(dir, fileName) : fileName
      await createFileNode(wsId, path, 'file')
      return path
    },
    handleCreateNewFolderInDirectory: async (dirPath: string, folderName?: string) => {
      if (!folderName) return
      const dir = relativeDir(dirPath)
      await createFileNode(wsId, dir ? joinPath(dir, folderName) : folderName, 'dir')
    },
    handleDeletePath: async (path: string) => {
      await deleteFileNode(wsId, path)
    },
    handleDuplicatePath: async (path: string) => {
      try {
        await copyFileNode(wsId, path, duplicateDestPath(path, store.getState().files))
      } catch (error) {
        report(error, { action: 'Duplicate failed' })
      }
    },
    handleRenamePath: async (path: string, newName?: string) => {
      if (newName) {
        const dir = parentDir(path)
        await renameFileNode(wsId, path, dir ? joinPath(dir, newName) : newName)
        return
      }
      const setRenaming = (nodes: AppFile[]): AppFile[] =>
        nodes.map((n) => {
          if (n.path === path) return { ...n, isRenaming: true, isEditing: true }
          return n.children ? { ...n, children: setRenaming(n.children) } : n
        })
      const { files, fileTreeActions } = store.getState()
      fileTreeActions.setFiles(setRenaming(files))
    },
    refreshDirectory: (path?: string) => refreshDirectory(wsId, store, path),
  }
}
