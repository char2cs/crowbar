import { useMemo, useEffect } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getExplorerTargetPath } from '@/features/file-explorer/utils/file-explorer-tree-utils'
import type { PaneContent } from '@/features/panes/types/pane-content'

interface UseFileExplorerSyncOptions {
  activePath?: string
  updateActivePath?: (path: string) => void
  revealPathInTree: (path: string) => void | Promise<void>
}

const EMPTY_BUFFERS: PaneContent[] = []

export function useFileExplorerSync({
  activePath,
  updateActivePath,
  revealPathInTree,
}: UseFileExplorerSyncOptions) {
  // Task 26: buffers/panes are window-level and never destroyed — a plain
  // zustand selector off the one singleton store, no more "active workspace"
  // resubscription (useActiveWorkspaceState) needed.
  const buffers = useStore(windowPaneStore, (s) => s.buffers) ?? EMPTY_BUFFERS
  const activeBufferId = useStore(
    windowPaneStore,
    (s) => s.paneActions.getActivePane()?.activeEditorTabId ?? null,
  )

  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) || null,
    [buffers, activeBufferId],
  )

  const explorerTargetPath = useMemo(() => getExplorerTargetPath(activeBuffer), [activeBuffer])

  useEffect(() => {
    if (!explorerTargetPath) {
      if (activePath) {
        updateActivePath?.('')
      }
      return
    }

    if (explorerTargetPath === activePath) return
    updateActivePath?.(explorerTargetPath)
  }, [activePath, explorerTargetPath, updateActivePath])

  useEffect(() => {
    if (!explorerTargetPath) return
    void revealPathInTree(explorerTargetPath)
  }, [explorerTargetPath, revealPathInTree])
}
