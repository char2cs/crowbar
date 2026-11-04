import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { fetchFileTree, findNode, mergeChildren } from '@/features/files/lib/file-tree-api'
import { refreshDirectory } from '@/features/files/lib/file-tree-handlers'
import {
  reconcileWorkspaceDocuments,
  syncBufferWithDisk,
} from '@/features/workspace/lib/external-buffer-sync'
import { workspaceRef } from '@/lib/api/documents'
import { holdScope, onSignal, watchScope } from '@/core/stream/stream'
import { report } from '@/lib/errors/report'
import { useResolvedHost } from '@/lib/host-resolve'
import { useWorkspaceStore } from '../workspace-context'
import { UserFacingError } from '@/lib/errors/user-facing-error'

// Stable reference for a workspace with nothing expanded yet — a fresh
// `new Set()` on every selector call would re-run the lazy-load effect each render.
const EMPTY_EXPANDED_PATHS: ReadonlySet<string> = new Set()

interface FileChangeEvent {
  type?: string
  path?: string
  newPath?: string
}

function parentDir(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

// A plain content edit ("modified") never changes the tree's shape, so it must
// not trigger a directory refetch. Only structural events touch the tree.
function isStructuralChange(type: string | undefined): boolean {
  return type === 'created' || type === 'deleted' || type === 'renamed'
}

function reconcileDocuments(wsId: string): void {
  void reconcileWorkspaceDocuments(wsId).catch((err: unknown) =>
    report(err, { action: 'Check open files against disk' }),
  )
}

/**
 * Keeps `wsId`'s own file tree (its workspace store's slice) loaded and live.
 *
 * The workspace's files: scope is held alongside the first read; a snapshot
 * landing during that read re-lists after it, so no change in between is lost.
 * It stays held while the workspace is retained: switching back never refetches.
 * Waits on `chatScopeReady` (non-home file routes are chat-scoped) and on the
 * workspace's host being known.
 */
export function useWorkspaceFileTree(wsId: string, active: boolean, chatScopeReady: boolean) {
  const store = useWorkspaceStore()
  const hostId = useResolvedHost({ wsId })
  const addressable = hostId !== null && chatScopeReady
  const status = useStore(store, (s) => s.fileTreeStatus)
  const files = useStore(store, (s) => s.files)
  const expandedPaths = useFileTreeStore(
    (state) => state.expandedPathsByWorkspace[wsId] ?? EMPTY_EXPANDED_PATHS,
  )
  const loadingDirs = useRef<Set<string>>(new Set())
  // How many snapshots of files: have landed: a read that saw one land meanwhile re-lists after.
  const syncs = useRef(0)
  const wanted = addressable && (active || status !== 'idle')

  // Holds the scope and applies its file changes. Every synced after the tree
  // is loaded may follow missed writes, so it re-reads the tree and open buffers.
  useEffect(() => {
    if (!wanted || !hostId) return
    const scope = `files:${wsId}`
    const stopWatch = watchScope(hostId, scope, {
      synced: () => {
        syncs.current++
        if (store.getState().fileTreeStatus !== 'ready') return
        void refreshDirectory(wsId, store)
        reconcileDocuments(wsId)
      },
      refused: (problem) => {
        store.getState().fileTreeActions.setFileTreeStatus('failed')
        report(new UserFacingError(problem?.message ?? `${scope} was refused`), {
          action: 'Follow the file tree',
        })
      },
    })
    const stopSignals = onSignal<FileChangeEvent>(hostId, scope, 'file_change', (evt) => {
      if (!evt?.path) return
      // An external write to an open buffer must reconcile it (silent reload
      // when clean, conflict flag when dirty); own-save echoes are filtered inside.
      if (evt.type === 'modified' || evt.type === 'created') {
        void syncBufferWithDisk(workspaceRef(wsId, evt.path))
      }
      if (!isStructuralChange(evt.type)) return
      void refreshDirectory(wsId, store, parentDir(evt.path))
      if (evt.newPath) void refreshDirectory(wsId, store, parentDir(evt.newPath))
    })
    const release = holdScope(hostId, scope)
    return () => {
      release()
      stopSignals()
      stopWatch()
    }
  }, [store, wsId, wanted, hostId])

  // The first read never waits on the stream, which may be down.
  useEffect(() => {
    if (!active || !addressable) return
    const { fileTreeStatus, fileTreeActions } = store.getState()
    if (fileTreeStatus === 'loading' || fileTreeStatus === 'ready') return
    fileTreeActions.setFileTreeStatus('loading')
    const seen = syncs.current
    void fetchFileTree(wsId).then(
      (root) => {
        if (store.getState().fileTreeStatus !== 'loading') return // refused meanwhile
        store.setState({ files: root, fileTreeStatus: 'ready' })
        if (syncs.current !== seen) void refreshDirectory(wsId, store)
        // Documents restored while this workspace was never shown may predate disk.
        reconcileDocuments(wsId)
      },
      (err: unknown) => {
        fileTreeActions.setFileTreeStatus('failed')
        report(err, { action: 'Load the file tree' })
      },
    )
  }, [store, wsId, active, addressable])

  // Lazily fetch a directory's children the first time it is expanded, and
  // re-check on every `files` change: an expanded folder whose children were
  // dropped by a root re-list must refetch even though `expandedPaths` is the
  // same reference.
  useEffect(() => {
    if (!addressable) return
    for (const path of expandedPaths) {
      const node = findNode(files, path)
      if (!node?.isDir || node.children !== undefined) continue
      if (loadingDirs.current.has(path)) continue
      loadingDirs.current.add(path)
      void fetchFileTree(wsId, path)
        .then((children) => {
          const { files: current, fileTreeActions } = store.getState()
          fileTreeActions.setFiles(mergeChildren(current, path, children))
        })
        .catch((err: unknown) => report(err, { action: 'Load the folder' }))
        .finally(() => loadingDirs.current.delete(path))
    }
  }, [store, wsId, expandedPaths, files, addressable])
}
