import { type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { useCallback, useState } from 'react'
import type { PaneContent } from '@/features/panes/types/pane-content'

interface UseTabDragOptions {
  sortedBuffers: PaneContent[]
  onTabSelect: (buffer: PaneContent) => void
  onTabClick: (bufferId: string) => void
  onReorderBuffers: (oldIndex: number, newIndex: number) => void
}

/** dnd-kit drag state and handlers for one tab bar; a drop never creates a split (pane groups
 *  group chats, not tabs). Each TabBar mounts its own DndContext, so `event.over` only names this
 *  pane's tabs: never resolve a drop past that (e.g. DOM hit-testing) or tabs cross panes. */
export function useTabDrag({
  sortedBuffers,
  onTabSelect,
  onTabClick,
  onReorderBuffers,
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

      // Resolved only among this pane's droppables, so the sole outcome is a reorder.
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
