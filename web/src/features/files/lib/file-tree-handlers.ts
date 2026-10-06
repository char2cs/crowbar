import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  copyFileNode,
  createFileNode,
  deleteFileNode,
  fetchFileTree,
  findNode,
  mergeChildren,
  renameFileNode,
} from '@/features/files/lib/file-tree-api'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AppFile } from '@/features/file-system/types/app'
import { revealItemInFinder } from '@/lib/crowbar-bridge'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { toast } from '@/features/window/stores/toast-store'
import { joinPath } from '@/utils/path-helpers'

/** Which pane a file opens into (C8). Omitted, the focused pane. */
export interface FileOpenTarget {
  paneId?: string
}

function parentDir(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

// Derive a non-colliding "<name> copy" destination for a duplicate, checking the
// loaded siblings so a second duplicate becomes "<name> copy 2" instead of
// clobbering the first. A dot in position 0 (dotfile) is treated as part of the
// stem, not an extension, so ".env" duplicates to ".env copy".
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

// Carry already-loaded children across a level refresh so a live file change
// does not collapse expanded subtrees that are still present.
function preserveLoadedChildren(current: AppFile[], fresh: AppFile[]): AppFile[] {
  return fresh.map((node) => {
    if (!node.isDir) return node
    const existing = findNode(current, node.path)
    if (existing?.children) return { ...node, children: existing.children }
    return node
  })
}

/** Re-list one directory level (the root when `path` is empty) into `store`'s tree. */
export async function refreshDirectory(
  wsId: string,
  store: WorkspaceStore,
  path?: string,
): Promise<void> {
  const fresh = await fetchFileTree(wsId, path || undefined).catch(() => null)
  if (!Array.isArray(fresh)) return
  const { files, fileTreeActions } = store.getState()
  const reconciled = preserveLoadedChildren(files, fresh)
  fileTreeActions.setFiles(!path ? reconciled : mergeChildren(files, path, reconciled))
}

/** Open a workspace-relative file in an editor tab (preview unless `preview` is false). */
export function openWorkspaceFile(
  wsId: string,
  path: string,
  opts: FileOpenTarget & { preview?: boolean } = {},
): Promise<void> {
  return openFileContent(wsId, path, windowPaneStore.getState().bufferActions, {
    preview: opts.preview ?? false,
    paneId: opts.paneId,
  })
}

// Reveal in Finder (explorer + tab context menus). The tab menu passes the
// buffer's workspace-relative path; the explorer passes an absolute one.
// Relative paths resolve against the on-disk workspace root; virtual buffers
// (diff:// …) have no disk presence to reveal.
export function revealInFolder(path: string): void {
  if (path.includes('://')) return
  const root = path.startsWith('/') ? '' : resolveWorkspaceRootPath()
  if (root === undefined) return
  const absolute = root ? joinPath(root, path) : path
  revealItemInFinder(absolute).catch((error: unknown) => {
    toast.error('Reveal in Finder failed', error instanceof Error ? error.message : String(error))
  })
}

/**
 * The file explorer's mutations for one workspace. The daemon emits a
 * structural FileChangeEvent on success, which the files stream reconciles into
 * the tree — so these don't refetch (except the explicit Refresh action).
 */
export function createFileTreeHandlers(wsId: string, store: WorkspaceStore) {
  // Tree paths are workspace-relative (root === ''); a dirPath equal to the
  // absolute wsId is the root addressed by its full path (right-click empty space).
  const relativeDir = (dirPath: string) => (dirPath === wsId ? '' : dirPath)
  return {
    handleFileOpen: async (path: string, isDir?: boolean, opts?: FileOpenTarget) => {
      if (isDir) return
      await openWorkspaceFile(wsId, path, { preview: false, paneId: opts?.paneId })
    },
    handleFileSelect: (path: string, isDir?: boolean, opts?: FileOpenTarget) => {
      if (isDir) return
      void openWorkspaceFile(wsId, path, { preview: true, paneId: opts?.paneId })
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
    // Duplicate goes through the daemon's server-side copy verb — byte faithful
    // and recursive for directories; only the collision-free destination is
    // derived client-side.
    handleDuplicatePath: async (path: string) => {
      try {
        await copyFileNode(wsId, path, duplicateDestPath(path, store.getState().files))
      } catch (error) {
        toast.error('Duplicate failed', error instanceof Error ? error.message : String(error))
      }
    },
    handleRenamePath: async (path: string, newName?: string) => {
      if (newName) {
        const dir = parentDir(path)
        await renameFileNode(wsId, path, dir ? joinPath(dir, newName) : newName)
        return
      }
      // No newName → START an inline rename: an idempotent SET (never a toggle)
      // so a double-click reliably opens the input whatever a prior edit left.
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
