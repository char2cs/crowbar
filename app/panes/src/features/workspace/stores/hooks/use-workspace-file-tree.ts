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
import { isNotFoundError } from '@/lib/errors/api-error'

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

function isStructuralChange(type: string | undefined): boolean {
  return type === 'created' || type === 'deleted' || type === 'renamed'
}

function reconcileDocuments(wsId: string): void {
  void reconcileWorkspaceDocuments(wsId).catch((err: unknown) =>
    report(err, { action: 'Check open files against disk' }),
  )
}

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
  const syncs = useRef(0)
  const wanted = addressable && (active || status !== 'idle')

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

  useEffect(() => {
    if (!active || !addressable) return
    const { fileTreeStatus, fileTreeActions } = store.getState()
    if (fileTreeStatus === 'loading' || fileTreeStatus === 'ready') return
    fileTreeActions.setFileTreeStatus('loading')
    const seen = syncs.current
    void fetchFileTree(wsId).then(
      (root) => {
        if (store.getState().fileTreeStatus !== 'loading') return
        store.setState({ files: root, fileTreeStatus: 'ready' })
        if (syncs.current !== seen) void refreshDirectory(wsId, store)
        reconcileDocuments(wsId)
      },
      (err: unknown) => {
        fileTreeActions.setFileTreeStatus('failed')
        report(err, { action: 'Load the file tree' })
      },
    )
  }, [store, wsId, active, addressable])

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
        .catch((err: unknown) => {
          if (isNotFoundError(err)) useFileTreeStore.getState().collapsePath(wsId, path)
          else report(err, { action: 'Load the folder' })
        })
        .finally(() => loadingDirs.current.delete(path))
    }
  }, [store, wsId, expandedPaths, files, addressable])
}
