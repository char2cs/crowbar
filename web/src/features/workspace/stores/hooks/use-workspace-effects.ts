import { useEffect, useRef } from 'react'
import deepEqual from 'fast-deep-equal'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import {
  copyFileNode,
  createFileNode,
  deleteFileNode,
  fetchFileTree,
  filesWsEndpoint,
  findNode,
  mergeChildren,
  renameFileNode,
} from '@/features/files/lib/file-tree-api'
import { joinPath } from '@/utils/path-helpers'
import { revealItemInFinder } from '@/lib/crowbar-bridge'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { toast } from '@/features/window/stores/toast-store'
import { wsManager } from '@/lib/ws/manager'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { gitBaseForWorkspace, isHomeWorkspace } from '@/lib/workspace-scope-url'
import { fetchAllGitData, useGitStore } from '@/features/git/stores/git-store'
import { useWorkspaceThreadsStream } from './use-workspace-threads-stream'
import {
  isWarmDataFresh,
  peekGitFrame,
  saveGitFrame,
} from '@/features/workspace/lib/activation-freshness'
import type { AppFile } from '@/features/file-system/types/app'

const GIT_REFRESH_DEBOUNCE_MS = 400

function parentDir(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

// Derive a non-colliding "<name> copy" destination for a duplicate, checking the
// loaded siblings so a second duplicate becomes "<name> copy 2" instead of
// clobbering the first. A dot in position 0 (dotfile) is treated as part of the
// stem, not an extension, so ".env" duplicates to ".env copy".
export function duplicateDestPath(srcPath: string, files: AppFile[]): string {
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

// The push stream repeats identical git/status frames far faster than the
// reload debounce, so only a frame that actually differs from the previous
// one should retrigger a reload. `prev` is `null` before the first frame of
// a session arrives — null must never compare equal to an incoming frame, or
// that first frame would silently fail to trigger a reload. Comparing the
// already-parsed frame objects with fast-deep-equal (walk + bail on first
// mismatch) avoids JSON.stringify-ing a multi-KB payload on every frame.
export function framesEqual(prev: unknown, next: unknown): boolean {
  return prev !== null && deepEqual(prev, next)
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

    // The workspace-scoped file handlers, wired into the global fs store on
    // every (re)seed. Built once per mount (closures over wsId/bufferActions) so
    // the full seed and the warm fast path install the exact same closures.
    const handlers = {
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
      // Reveal in Finder (explorer + tab context menus). The tab menu passes
      // the buffer's workspace-relative path; the explorer passes an absolute
      // one (already joined with the worktree root). Resolve relative paths
      // against the on-disk workspace root; virtual buffers (remote://,
      // diff:// …) have no disk presence to reveal. Failures surface as a
      // toast instead of vanishing into an uncaught rejection.
      handleRevealInFolder: (path: string) => {
        if (path.includes('://')) return
        const root = path.startsWith('/') ? '' : resolveWorkspaceRootPath()
        if (root === undefined) return
        const absolute = root ? joinPath(root, path) : path
        revealItemInFinder(absolute).catch((error: unknown) => {
          toast.error(
            'Reveal in Finder failed',
            error instanceof Error ? error.message : String(error),
          )
        })
      },
      // Duplicate goes through the daemon's server-side copy verb — byte
      // faithful (binary files stay intact) and recursive for directories.
      // Only the collision-free "<name> copy" destination is derived
      // client-side; the daemon's structural FileChangeEvent reconciles the
      // new node into the tree. Failures (locked workspace 409, missing
      // source 404 …) surface as a toast.
      handleDuplicatePath: async (path: string) => {
        const files = useFileSystemStore.getState().files
        try {
          await copyFileNode(wsId, path, duplicateDestPath(path, files))
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
    }

    // Warm fast path: this workspace was hidden only briefly AND the global tree
    // still holds ITS data (no other workspace clobbered it while away) — keep
    // the loaded tree, skip the refetch and the `isFileTreeLoading:true` flash
    // (the synchronous re-render that inflated the warm-switch frame). Re-wire
    // the handlers only if they were somehow dropped. See activation-freshness.
    //
    // The `!isFileTreeLoading` clause is LOAD-BEARING for the real A→B→A path:
    // resetWorkspaceScopedStores (activation layout effect) normalises
    // rootFolderPath back to the returning wsId with an EMPTY tree and
    // isFileTreeLoading:true, so the identity check alone would happily "keep"
    // that empty tree. Mid-load means the store holds no real data — re-seed.
    const fs0 = useFileSystemStore.getState()
    if (fs0.rootFolderPath === wsId && !fs0.isFileTreeLoading && isWarmDataFresh(wsId)) {
      if (!fs0.handleFileOpen) useFileSystemStore.setState(handlers)
      return () => {
        cancelled = true
      }
    }

    // Cold / stale seed. Reset the shared tree synchronously on switch so the
    // user never sees the previous workspace's files while the new tree loads.
    if (fs0.rootFolderPath !== wsId) {
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
        ...handlers,
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
        void syncBufferWithDisk(wsId, evt.path)
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
  // The home (project-level) workspace has no git surface: the project root is
  // not a git worktree at all, so there is nothing for status/log/branches to
  // answer. That was true when git was addressed by workspace and is still true
  // now that it is addressed by chat — the route changed, the absence of a
  // worktree did not. Skip all git loading and the git/status stream for it.
  // Files and threads remain enabled for home.
  useEffect(() => {
    if (isHomeWorkspace(wsId)) return
    let cancelled = false

    // Warm fast path: git data for THIS workspace survived a brief hide — skip
    // the four-request initial load. Status/log still self-heal below (the
    // re-subscribed stream reloads on any frame that differs from the one
    // preserved across the gap); branches/stashes change only on explicit git
    // actions, so keeping the loaded values is correct.
    const gitFresh =
      useGitStore.getState().currentWorkspaceRepoPath === wsId && isWarmDataFresh(wsId)
    if (!gitFresh) {
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
    }
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
    // actually differs from the previous one warrants a refetch. Seed from the
    // frame preserved across the hidden gap so a re-subscribe that re-pushes the
    // SAME status doesn't retrigger a reload; a differing frame still does (the
    // self-heal for a change that landed while hidden). Null on a cold seed.
    let lastFrame: unknown = gitFresh ? peekGitFrame(wsId) : null
    const unsubscribe = wsManager.subscribe(`${gitBaseForWorkspace(wsId)}/status`, (frame) => {
      if (framesEqual(lastFrame, frame)) return
      lastFrame = frame
      scheduleStatusReload()
    })
    // Editor saves dispatch "git-status-updated" after a successful write.
    // Refresh on it directly so the Changes panel updates deterministically,
    // without depending on the backend watcher's git event arriving.
    window.addEventListener('git-status-updated', scheduleStatusReload)
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      window.removeEventListener('git-status-updated', scheduleStatusReload)
      unsubscribe()
      // Preserve the last-seen frame so a quick warm return dedupes the re-push.
      saveGitFrame(wsId, lastFrame)
    }
  }, [wsId])
}
