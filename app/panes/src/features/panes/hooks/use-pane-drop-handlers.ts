import { useCallback, useState, type RefObject } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useDragStore } from '@/features/panes/stores/drag-store'
import { openDocumentsAtPaths } from '@/features/editor/documents/open-document'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { isTauri } from '@/lib/crowbar-bridge'
import { ensureBufferInPaneDropTarget } from '@/features/panes/utils/pane-drop-actions'
import type { DropZone } from '@/features/panes/components/split-drop-overlay'

interface TabDragPayload {
  bufferId?: string
  paneId?: string
}

function readTabPayload(e: React.DragEvent): TabDragPayload | null {
  const raw = e.dataTransfer.getData('application/tab-data')
  if (!raw) return null
  try {
    return JSON.parse(raw) as TabDragPayload
  } catch {
    return null
  }
}

/** A pane's drags: file ring, tab split overlay, sidebar/explorer hover, and their drops. A tab
 *  never crosses panes or makes a split (panes group chats, not tabs): only its own tab lands. */
export function usePaneDropHandlers(paneId: string, containerRef: RefObject<HTMLElement | null>) {
  const [isDragOver, setIsDragOver] = useState(false)
  const [isTabDragOver, setIsTabDragOver] = useState(false)
  // A narrow selector: only the hovered pane re-renders.
  const internalHoverZone = useDragStore((s): DropZone =>
    s.hover.paneId === paneId ? s.hover.zone : null,
  )

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const types = e.dataTransfer.types
    const hasTabData = types.includes('application/tab-data')
    // An OS file drag lands only on the desktop: a browser drop carries no host path.
    const accepts =
      hasTabData ||
      types.includes('text/plain') ||
      (types.includes('Files') && isTauri()) ||
      !!useDragStore.getState().file
    if (!accepts) return
    e.dataTransfer.dropEffect = 'move'
    setIsDragOver(true)
    if (hasTabData) setIsTabDragOver(true)
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const relatedTarget = e.relatedTarget as HTMLElement | null
    if (!relatedTarget || !(e.currentTarget as HTMLElement).contains(relatedTarget)) {
      setIsDragOver(false)
      setIsTabDragOver(false)
    }
  }, [])

  const handleSplitDrop = useCallback(
    (zone: DropZone, e: React.DragEvent) => {
      setIsDragOver(false)
      setIsTabDragOver(false)
      if (!zone) return
      const tab = readTabPayload(e)
      if (!tab) return
      const { bufferId } = tab
      if (!bufferId || (tab.paneId && tab.paneId !== paneId)) return
      const actions = windowPaneStore.getState().paneActions
      if (zone !== 'center') {
        actions.activateEditorTabInPane(paneId, bufferId)
        return
      }
      ensureBufferInPaneDropTarget(bufferId, { paneId, zone: 'center' })
      const buffer = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
      if (buffer) actions.addEditorTabToPane(paneId, buffer)
    },
    [paneId],
  )

  // Tab drops are handled by SplitDropOverlay; an OS file drop arrives through Tauri.
  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragOver(false)
    setIsTabDragOver(false)
  }, [])

  const handleTauriFileDrop = useCallback(
    (paths: string[]) => openDocumentsAtPaths(paths, paneId),
    [paneId],
  )
  useTauriFileDrop(containerRef, handleTauriFileDrop)

  return {
    isDragOver,
    isTabDragOver,
    internalHoverZone,
    handleDragOver,
    handleDragLeave,
    handleSplitDrop,
    handleDrop,
  }
}
