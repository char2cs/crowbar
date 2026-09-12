import { type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { useCallback, useState } from 'react'
import type { PaneContent } from '@/features/panes/types/pane-content'

interface UseTabDragOptions {
  sortedBuffers: PaneContent[]
  onTabSelect: (buffer: PaneContent) => void
  onTabClick: (bufferId: string) => void
  onReorderBuffers: (oldIndex: number, newIndex: number) => void
  onSplitPane: (
    targetPaneId: string,
    direction: 'horizontal' | 'vertical',
    bufferId?: string,
    placement?: 'before' | 'after',
  ) => string | undefined
}

/**
 * Encapsulates dnd-kit drag state and handlers for a tab bar.
 * Returns `draggedBufferId`, `draggedBuffer`, and the DndContext callbacks.
 *
 * A tab can only ever be reordered within ITS OWN pane's tab bar. Each
 * TabBar mounts its own `DndContext`/`SortableContext` (tab-bar.tsx), so
 * `event.over` here can never name a droppable from another pane's tab bar —
 * dnd-kit scopes collision detection to droppables registered under the same
 * `DndContext`. That isolation is the whole guarantee: this hook must never
 * reach past it (e.g. by hit-testing the DOM under the pointer to find
 * whatever pane happens to be there) to resolve a drop target itself, or the
 * isolation is void and a tab can cross into another pane's tab bar again.
 */
export function useTabDrag({
  sortedBuffers,
  onTabSelect,
  onTabClick,
  onReorderBuffers,
  // onSplitPane is still accepted (tab-bar.tsx keeps wiring it from
  // paneActions.splitPane) but deliberately unused: spec §7.3 — "a pane
  // group is a group of chats, never of tabs" (Law 3) — dropping a dragged
  // tab must never create a new pane/split any more, so handleDragEnd below
  // no longer calls it.
}: UseTabDragOptions) {
  const [draggedBufferId, setDraggedBufferId] = useState<string | null>(null)

  const draggedBuffer =
    draggedBufferId != null ? (sortedBuffers.find((b) => b.id === draggedBufferId) ?? null) : null

  const resetDrag = useCallback(() => {
    setDraggedBufferId(null)
  }, [])

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const buffer = sortedBuffers.find((item) => item.id === String(event.active.id))
      if (!buffer) return

      setDraggedBufferId(buffer.id)
      onTabSelect(buffer)
    },
    [onTabSelect, sortedBuffers],
  )

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const activeId = String(event.active.id)
      const dragged = sortedBuffers.find((buffer) => buffer.id === activeId)

      // `event.over` is dnd-kit's own collision result, resolved only among
      // droppables registered in THIS pane's SortableContext — reordering
      // within the pane's own tab bar is the only outcome this can ever
      // produce.
      if (event.over) {
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
    [onTabClick, onReorderBuffers, resetDrag, sortedBuffers],
  )

  return {
    draggedBufferId,
    draggedBuffer,
    handleDragStart,
    handleDragEnd,
    resetDrag,
  }
}
