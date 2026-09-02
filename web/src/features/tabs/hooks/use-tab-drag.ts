import { type DragEndEvent, type DragMoveEvent, type DragStartEvent } from '@dnd-kit/core'
import { useCallback, useEffect, useRef, useState } from 'react'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useUIState } from '@/features/window/stores/ui-state-store'
import {
  clearInternalTabDragData,
  resolveDropTarget,
  setInternalTabDragHoverTarget,
  setInternalTabDragData,
} from '../utils/internal-tab-drag'

const getClientPoint = (event: Event) => {
  const candidate = event as Partial<MouseEvent>
  if (typeof candidate.clientX === 'number' && typeof candidate.clientY === 'number') {
    return { x: candidate.clientX, y: candidate.clientY }
  }
  return null
}

interface UseTabDragOptions {
  paneId: string | undefined
  sortedBuffers: PaneContent[]
  onTabSelect: (buffer: PaneContent) => void
  onTabClick: (bufferId: string) => void
  onReorderBuffers: (oldIndex: number, newIndex: number) => void
  onMoveBufferToPane: (bufferId: string, fromPaneId: string, toPaneId: string) => void
  onActivatePaneBuffer: (paneId: string, bufferId: string) => void
  onSplitPane: (
    targetPaneId: string,
    direction: 'horizontal' | 'vertical',
    bufferId?: string,
    placement?: 'before' | 'after',
  ) => string | undefined
}

/**
 * Encapsulates all dnd-kit drag state and handlers for the tab bar.
 * Returns `draggedBufferId`, `draggedBuffer`, and the four DndContext callbacks.
 */
export function useTabDrag({
  paneId,
  sortedBuffers,
  onTabSelect,
  onTabClick,
  onReorderBuffers,
  onMoveBufferToPane,
  onActivatePaneBuffer,
  // onSplitPane is still accepted (tab-bar.tsx keeps wiring it from
  // paneActions.splitPane) but deliberately unused: spec §7.3 — "a pane
  // group is a group of chats, never of tabs" (Law 3) — dropping a dragged
  // tab must never create a new pane/split any more, so handleDragEnd below
  // no longer calls it.
}: UseTabDragOptions) {
  const [draggedBufferId, setDraggedBufferId] = useState<string | null>(null)
  const dragPointRef = useRef<{ x: number; y: number } | null>(null)
  const pointerPointRef = useRef<{ x: number; y: number } | null>(null)

  const draggedBuffer =
    draggedBufferId != null ? (sortedBuffers.find((b) => b.id === draggedBufferId) ?? null) : null

  const getDragPoint = (event: DragMoveEvent | DragEndEvent) => {
    if (pointerPointRef.current) return pointerPointRef.current
    const rect = event.active.rect.current.translated ?? event.active.rect.current.initial
    if (!rect) return dragPointRef.current
    return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
  }

  const resetDrag = useCallback(() => {
    setDraggedBufferId(null)
    dragPointRef.current = null
    pointerPointRef.current = null
    clearInternalTabDragData()
  }, [])

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const buffer = sortedBuffers.find((item) => item.id === String(event.active.id))
      if (!buffer) return

      setDraggedBufferId(buffer.id)
      pointerPointRef.current = getClientPoint(event.activatorEvent)
      setInternalTabDragData({ source: 'pane', bufferId: buffer.id, paneId })
      onTabSelect(buffer)
    },
    [onTabSelect, paneId, sortedBuffers],
  )

  const handleDragMove = useCallback(
    (event: DragMoveEvent) => {
      const point = getDragPoint(event)
      if (!point) return

      dragPointRef.current = point

      // Update cross-pane hover state whenever the pointer is over a different pane
      // or a split zone of any pane. The old isPointOutsideTabBar gate broke horizontal
      // splits: both tab bars sit at the same Y, so verticalSlop=64 never triggered.
      const dropTarget = resolveDropTarget(point)
      if (
        dropTarget.paneId !== null &&
        (dropTarget.paneId !== paneId || dropTarget.zone !== 'center')
      ) {
        setInternalTabDragHoverTarget(dropTarget)
      } else {
        // Hovering over the source tab bar (reorder mode) — clear any stale indicator
        setInternalTabDragHoverTarget({ paneId: null, zone: null })
      }
    },
    [paneId],
  )

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const activeId = String(event.active.id)
      const dragged = sortedBuffers.find((buffer) => buffer.id === activeId)
      const point = getDragPoint(event)
      const target = point ? resolveDropTarget(point) : { paneId: null, zone: null }

      // Spec §7.3: "a pane group is a group of chats, never of tabs" (Law 3)
      // — dropping a dragged tab must never create a new pane/split, so an
      // edge zone no longer routes through onSplitPane. A drop on a
      // DIFFERENT pane — center or edge alike — just moves the tab into
      // that pane's existing tab group. A drop back on this tab's own pane
      // (its edge, with nothing to reorder against) falls through to the
      // reorder branch below, which no-ops when there is nothing under the
      // pointer to reorder onto.
      if (dragged && paneId && target.paneId && target.paneId !== paneId) {
        const destinationPaneId = target.paneId
        onMoveBufferToPane(dragged.id, paneId, destinationPaneId)
        onActivatePaneBuffer(destinationPaneId, dragged.id)
        if (destinationPaneId === BOTTOM_PANE_ID) {
          useUIState.getState().setBottomPaneActiveTab('buffers')
          useUIState.getState().setIsBottomPaneVisible(true)
        }
      } else if (event.over) {
        const oldIndex = sortedBuffers.findIndex((buffer) => buffer.id === activeId)
        const newIndex = sortedBuffers.findIndex((buffer) => buffer.id === String(event.over?.id))
        if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
          onReorderBuffers(oldIndex, newIndex)
          if (dragged) {
            onTabClick(dragged.id)
          }
        }
      }

      resetDrag()
    },
    [onActivatePaneBuffer, onTabClick, onMoveBufferToPane, paneId, onReorderBuffers, resetDrag, sortedBuffers],
  )

  // Track pointer position during drag for accurate drop-target resolution
  useEffect(() => {
    if (!draggedBufferId) return

    const updatePointerPoint = (event: PointerEvent) => {
      pointerPointRef.current = { x: event.clientX, y: event.clientY }
    }

    window.addEventListener('pointermove', updatePointerPoint, true)
    return () => window.removeEventListener('pointermove', updatePointerPoint, true)
  }, [draggedBufferId])

  return {
    draggedBufferId,
    draggedBuffer,
    handleDragStart,
    handleDragMove,
    handleDragEnd,
    resetDrag,
  }
}
