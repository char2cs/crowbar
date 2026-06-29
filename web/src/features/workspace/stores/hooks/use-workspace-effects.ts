import { useEffect, useRef } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import {
  createFileNode,
  deleteFileNode,
  fetchFileTree,
  filesWsEndpoint,
  findNode,
  mergeChildren,
  renameFileNode,
} from '@/features/files/lib/file-tree-api'
import { joinPath } from '@/utils/path-helpers'
import { wsManager } from '@/lib/ws/manager'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceBase, isHomeWorkspace } from '@/lib/workspace-scope-url'
import { fetchAllGitData, useGitStore } from '@/features/git/stores/git-store'
import { useWorkspaceThreadsStream } from './use-workspace-threads-stream'
import type { AppFile } from '@/features/file-system/types/app'

const GIT_REFRESH_DEBOUNCE_MS = 400

function parentDir(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
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

interface FileChangeEvent {
  type?: string
  path?: string
  newPath?: string
}

// A plain content edit ("modified") never changes the tree's shape, so it must
// not trigger a directory refetch — that was the per-keystroke churn that made
// editing feel slow. Only structural events touch the tree.
function isStructuralChange(type: string | undefined): boolean {
  return type === 'created' || type === 'deleted' || type === 'renamed'
}

export function useWorkspaceEffects(wsId: string) {
  const bufferActions = useBufferActions()
  const expandedPaths = useFileTreeStore((state) => state.expandedPaths)
  const loadingDirs = useRef<Set<string>>(new Set())

  useWorkspaceThreadsStream(wsId)

  // Seed the root file tree and wire the workspace-scoped file handlers. The
  // `cancelled` guard ensures a slow fetch from a previous workspace cannot
  // overwrite the global file-system store after the user has switched away.
  useEffect(() => {
    let cancelled = false
    loadingDirs.current.clear()
    // Reset the shared tree synchronously on switch so the user never sees the
    // previous workspace's files while the new tree loads.
    if (useFileSystemStore.getState().rootFolderPath !== wsId) {
      useFileSystemStore.setState({
        rootFolderPath: wsId,
        files: [],
        fileTree: [],
        isFileTreeLoading: true,
      })
    } else {
      useFileSystemStore.setState({ isFileTreeLoading: true })
    }
    void (async () => {
      const root = await fetchFileTree(wsId).catch(() => null)
      if (cancelled) return
      if (!Array.isArray(root)) {
        useFileSystemStore.setState({ isFileTreeLoading: false })
        return
      }
      useFileSystemStore.setState({
        rootFolderPath: wsId,
        files: root,
        fileTree: root,
        isFileTreeLoading: false,
        handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
          if (revealOrIsDir === true) return
          await openFileContent(wsId, path, bufferActions, { preview: false })
        },
        handleFileSelect: (path: string, isDir?: boolean) => {
          if (isDir) return
          void openFileContent(wsId, path, bufferActions, { preview: true })
        },
        // File-tree mutations. The daemon emits a structural FileChangeEvent on
        // success, which the files-WS effect below reconciles into the tree — so
        // these don't refetch (except the explicit Refresh action).
        handleCreateNewFileInDirectory: async (dirPath: string, fileName?: string) => {
          if (!fileName) return
          // Tree paths are workspace-relative (root === ''). A dirPath equal to
          // the absolute wsId is the workspace root addressed by its full path
          // (right-click empty space) — normalise it to '' so we create at the
          // worktree root rather than at wsId/<name>.
          const dir = dirPath === wsId ? '' : dirPath
          const path = dir ? joinPath(dir, fileName) : fileName
          await createFileNode(wsId, path, 'file')
          return path
        },
        handleCreateNewFolderInDirectory: async (dirPath: string, folderName?: string) => {
          if (!folderName) return
          const dir = dirPath === wsId ? '' : dirPath
          await createFileNode(wsId, dir ? joinPath(dir, folderName) : folderName, 'dir')
        },
        handleDeletePath: async (path: string) => {
          await deleteFileNode(wsId, path)
        },
        handleRenamePath: async (path: string, newName?: string) => {
          if (newName) {
            const dir = parentDir(path)
            await renameFileNode(wsId, path, dir ? joinPath(dir, newName) : newName)
            return
          }
          // No newName → START an inline rename: mark the node editable. This is
          // an idempotent SET (never a toggle) so a double-click reliably opens
          // the input regardless of any stale isRenaming left by a prior edit —
          // that toggle-vs-stale-state coupling is what made rename inconsistent.
          // Cancel (Escape) and commit (Enter/blur) clear the flag explicitly in
          // the inline-editing hook, so start and clear are independent.
          const setRenaming = (nodes: AppFile[]): AppFile[] =>
            nodes.map((n) => {
              if (n.path === path) return { ...n, isRenaming: true, isEditing: true }
              return n.children ? { ...n, children: setRenaming(n.children) } : n
            })
          const fs = useFileSystemStore.getState()
          fs.setFiles(setRenaming(fs.files))
        },
        refreshDirectory: async (path?: string) => {
          const fresh = await fetchFileTree(wsId, path || undefined).catch(() => null)
          if (!Array.isArray(fresh)) return
          const current = useFileSystemStore.getState().files
          const reconciled = preserveLoadedChildren(current, fresh)
          const next = !path ? reconciled : mergeChildren(current, path, reconciled)
          useFileSystemStore.getState().setFiles(next)
        },
      })
    })()
    return () => {
      cancelled = true
    }
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Lazily fetch a directory's children the first time it is expanded.
  useEffect(() => {
    let cancelled = false
    for (const path of expandedPaths) {
      const node = findNode(useFileSystemStore.getState().files, path)
      if (!node?.isDir || node.children !== undefined) continue
      if (loadingDirs.current.has(path)) continue
      loadingDirs.current.add(path)
      void fetchFileTree(wsId, path)
        .then((children) => {
          if (cancelled) return
          const merged = mergeChildren(useFileSystemStore.getState().files, path, children)
          useFileSystemStore.getState().setFiles(merged)
        })
        .catch(() => {})
        .finally(() => loadingDirs.current.delete(path))
    }
    return () => {
      cancelled = true
    }
  }, [wsId, expandedPaths])

  // Apply live file-change events: refresh the affected directory level(s) in
  // place, preserving any expanded subtrees that still exist. Content-only
  // edits are ignored (the tree shape is unchanged).
  useEffect(() => {
    let cancelled = false
    const refreshDir = async (dir: string) => {
      const fresh = await fetchFileTree(wsId, dir || undefined).catch(() => null)
      if (cancelled || !Array.isArray(fresh)) return
      const current = useFileSystemStore.getState().files
      const reconciled = preserveLoadedChildren(current, fresh)
      const next = dir === '' ? reconciled : mergeChildren(current, dir, reconciled)
      useFileSystemStore.getState().setFiles(next)
    }
    const unsubscribe = wsManager.subscribe(filesWsEndpoint(wsId), (raw) => {
      const evt = raw as FileChangeEvent
      if (!evt?.path) return
      // An external write to a file with an open buffer must reconcile the
      // buffer (silent reload when clean, conflict flag when dirty) so a
      // later save cannot resurrect content that was discarded on disk.
      // Own-save echoes are filtered inside via the pending-save markers.
      if (evt.type === 'modified' || evt.type === 'created') {
        void syncBufferWithDisk(getOrCreateWorkspaceStore(wsId), evt.path)
      }
      if (!isStructuralChange(evt.type)) return
      void refreshDir(parentDir(evt.path))
      if (evt.newPath) void refreshDir(parentDir(evt.newPath))
    })
    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId])

  // Load full git data once, then keep status + commit log live on the git
  // topic. Branches/stashes change rarely — those reload on explicit git
  // actions.
  //
  // The home (project-level) workspace has no git surface — the backend mounts
  // no /home/git/* routes (the project root is not a per-workspace git
  // worktree). Skip all git loading and the git/status stream for it so we never
  // fire requests that 404. Files and threads remain enabled for home.
  useEffect(() => {
    if (isHomeWorkspace(wsId)) return
    let cancelled = false
    void (async () => {
      const data = await fetchAllGitData(wsId).catch(() => null)
      if (cancelled || !data) return
      useGitStore.getState().actions.loadFreshGitData({
        gitStatus: data.status,
        commits: data.commits,
        branches: data.branches,
        stashes: data.stashes,
        repoPath: wsId,
      })
    })()
    // Coalescing (non-resetting) timer. The backend can stream git frames more
    // often than the debounce window (observed ~165ms apart, indefinitely); a
    // resetting debounce starves forever under that load and the Changes panel
    // never refreshes. Coalescing guarantees a reload fires within the window
    // of the FIRST trigger, no matter how many more arrive meanwhile.
    let timer: ReturnType<typeof setTimeout> | null = null
    const scheduleStatusReload = () => {
      if (timer) return
      timer = setTimeout(() => {
        timer = null
        if (cancelled) return
        // Status + commit log together: terminal-side commits/resets change
        // the History list without any UI action (BUG-020). After the store
        // refresh, notify open diff views ("Uncommitted Changes" tab,
        // single-file diff tabs) so they refetch instead of showing the
        // already-committed hunks (BUG-017).
        void useGitStore
          .getState()
          .actions.reloadStatusAndLog(wsId)
          .then(() => {
            if (!cancelled) window.dispatchEvent(new CustomEvent('git-status-changed'))
          })
          .catch(() => {})
      }, GIT_REFRESH_DEBOUNCE_MS)
    }
    // The push stream repeats identical status frames; only a frame that
    // actually differs from the previous one warrants a refetch.
    let lastFrame: string | null = null
    const unsubscribe = wsManager.subscribe(
      `${workspaceBase(wsId)}/git/status`,
      (frame) => {
        let key: string
        try {
          key = JSON.stringify(frame)
        } catch {
          key = String(frame)
        }
        if (key === lastFrame) return
        lastFrame = key
        scheduleStatusReload()
      },
    )
    // Editor saves dispatch "git-status-updated" after a successful write.
    // Refresh on it directly so the Changes panel updates deterministically,
    // without depending on the backend watcher's git event arriving.
    window.addEventListener('git-status-updated', scheduleStatusReload)
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      window.removeEventListener('git-status-updated', scheduleStatusReload)
      unsubscribe()
    }
  }, [wsId])
}
