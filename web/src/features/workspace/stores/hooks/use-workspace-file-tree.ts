import { useEffect, useRef, useState } from 'react'
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
import { useWorkspaceStore } from '../workspace-context'

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
 * The workspace's files: scope is held before the tree is first fetched, so no
 * change between the read and the attach is lost, and stays held for as long as
 * the workspace is retained: switching back is a selection, never a refetch.
 * Waits on `chatScopeReady`: non-home file routes are chat-scoped.
 */
export function useWorkspaceFileTree(wsId: string, active: boolean, chatScopeReady: boolean) {
  const store = useWorkspaceStore()
  const status = useStore(store, (s) => s.fileTreeStatus)
  const files = useStore(store, (s) => s.files)
  const expandedPaths = useFileTreeStore(
    (state) => state.expandedPathsByWorkspace[wsId] ?? EMPTY_EXPANDED_PATHS,
  )
  const loadingDirs = useRef<Set<string>>(new Set())
  const [attached, setAttached] = useState(false)
  const wanted = chatScopeReady && (active || status !== 'idle')

  // Holds the scope and applies its file changes; a later synced is a resubscribe,
  // after which the tree and open buffers may have missed writes.
  useEffect(() => {
    if (!wanted) return
    const scope = `files:${wsId}`
    let first = true
    const stopWatch = watchScope(scope, {
      synced: () => {
        if (first) return void ((first = false), setAttached(true))
        void refreshDirectory(wsId, store)
        reconcileDocuments(wsId)
      },
    })
    const stopSignals = onSignal<FileChangeEvent>(scope, 'file_change', (evt) => {
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
    const release = holdScope(scope)
    return () => {
      release()
      stopSignals()
      stopWatch()
      setAttached(false)
    }
  }, [store, wsId, wanted])

  useEffect(() => {
    if (!active || !chatScopeReady || !attached) return
    const { fileTreeStatus, fileTreeActions } = store.getState()
    if (fileTreeStatus === 'loading' || fileTreeStatus === 'ready') return
    fileTreeActions.setFileTreeStatus('loading')
    void fetchFileTree(wsId).then(
      (root) => {
        store.setState({ files: root, fileTreeStatus: 'ready' })
        // Documents restored while this workspace was never shown may predate disk.
        reconcileDocuments(wsId)
      },
      (err: unknown) => {
        fileTreeActions.setFileTreeStatus('failed')
        report(err, { action: 'Load the file tree' })
      },
    )
  }, [store, wsId, active, chatScopeReady, attached])

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
        .catch((err: unknown) => report(err, { action: 'Load the folder' }))
        .finally(() => loadingDirs.current.delete(path))
    }
  }, [store, wsId, expandedPaths, files, chatScopeReady])
}
