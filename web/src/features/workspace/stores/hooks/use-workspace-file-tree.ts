import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import {
  fetchFileTree,
  filesWsEndpoint,
  findNode,
  mergeChildren,
} from '@/features/files/lib/file-tree-api'
import { refreshDirectory } from '@/features/files/lib/file-tree-handlers'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { reconcileWorkspaceBuffersWithDisk } from '@/lib/persistence/hydrate'
import { wsManager } from '@/lib/ws/manager'
import { useWorkspaceStore } from '../workspace-context'

// Stable reference for a workspace with nothing expanded yet — a fresh
// `new Set()` on every selector call would re-run the lazy-load effect each render.
const EMPTY_EXPANDED_PATHS: ReadonlySet<string> = new Set()

interface FileChangeEvent {
  reconnected?: boolean
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

/**
 * Keeps `wsId`'s own file tree (its workspace store's slice) loaded and live.
 *
 * The tree is fetched the first time the workspace is shown and then stays
 * subscribed to the files stream for as long as the workspace is retained, so
 * switching back is a selection, never a reset or a refetch. Waits on
 * `chatScopeReady`: non-home file routes are chat-scoped and throw without an
 * owning chat id.
 */
export function useWorkspaceFileTree(wsId: string, active: boolean, chatScopeReady: boolean) {
  const store = useWorkspaceStore()
  const status = useStore(store, (s) => s.fileTreeStatus)
  const files = useStore(store, (s) => s.files)
  const expandedPaths = useFileTreeStore(
    (state) => state.expandedPathsByWorkspace[wsId] ?? EMPTY_EXPANDED_PATHS,
  )
  const loadingDirs = useRef<Set<string>>(new Set())

  useEffect(() => {
    if (!active || !chatScopeReady) return
    const { fileTreeStatus, fileTreeActions } = store.getState()
    if (fileTreeStatus === 'loading' || fileTreeStatus === 'ready') return
    fileTreeActions.setFileTreeStatus('loading')
    void fetchFileTree(wsId)
      .catch(() => null)
      .then((root) => {
        if (!Array.isArray(root)) {
          fileTreeActions.setFileTreeStatus('failed')
          return
        }
        store.setState({ files: root, fileTreeStatus: 'ready' })
        // Buffers restored while this workspace was never shown may predate disk.
        void reconcileWorkspaceBuffersWithDisk(wsId).catch(() => {})
      })
  }, [store, wsId, active, chatScopeReady])

  // Lazily fetch a directory's children the first time it is expanded, and
  // re-check on every `files` change: an expanded folder whose children were
  // dropped by a root re-list must refetch even though `expandedPaths` is the
  // same reference.
  useEffect(() => {
    if (!chatScopeReady) return
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
        .catch(() => {})
        .finally(() => loadingDirs.current.delete(path))
    }
  }, [store, wsId, expandedPaths, files, chatScopeReady])

  // Apply live file-change events in place, preserving expanded subtrees that
  // still exist. The stream carries no snapshot on subscribe, which is why it
  // stays attached once the tree has loaded rather than reattaching per switch.
  const seeded = status !== 'idle'
  useEffect(() => {
    if (!chatScopeReady || !seeded) return
    return wsManager.subscribe(filesWsEndpoint(wsId), (raw) => {
      const evt = raw as FileChangeEvent
      // Writes may have been missed while the socket was down.
      if (evt?.reconnected) {
        void refreshDirectory(wsId, store)
        void reconcileWorkspaceBuffersWithDisk(wsId).catch(() => {})
        return
      }
      if (!evt?.path) return
      // An external write to an open buffer must reconcile it (silent reload
      // when clean, conflict flag when dirty); own-save echoes are filtered inside.
      if (evt.type === 'modified' || evt.type === 'created') {
        void syncBufferWithDisk(wsId, evt.path)
      }
      if (!isStructuralChange(evt.type)) return
      void refreshDirectory(wsId, store, parentDir(evt.path))
      if (evt.newPath) void refreshDirectory(wsId, store, parentDir(evt.newPath))
    })
  }, [store, wsId, chatScopeReady, seeded])
}
