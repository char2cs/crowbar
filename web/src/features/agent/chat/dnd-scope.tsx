import { createContext, type ReactNode, useCallback, useContext, useMemo, useState } from 'react'
import {
  DndContext,
  type DragEndEvent,
  type DragMoveEvent,
  PointerSensor,
  pointerWithin,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import {
  applyAttachmentDrop,
  type AttachmentDropTarget,
  AttachmentDropTargetContext,
  resolveAttachmentDrop,
} from '@/features/agent/composer/plate/attachment-drag-handle'

// Pointer-only (no keyboard sensor): the default instructions describe keys
// that do nothing here.
const ACCESSIBILITY = {
  screenReaderInstructions: { draggable: 'Drag the handle to move this attachment.' },
}

// Module-level so `useSensor` memoizes: a fresh options object per render makes
// dnd-kit mint new context values, and every consumer under the scope re-checks.
const POINTER_SENSOR_OPTIONS = { activationConstraint: { distance: 4 } }

function sameTarget(a: AttachmentDropTarget | null, b: AttachmentDropTarget | null) {
  return a?.id === b?.id && a?.line === b?.line
}

const ScopeChildren = createContext<ReactNode>(null)

function ScopeChildrenOutlet() {
  return useContext(ScopeChildren)
}

// One attachment-reorder scope per chat view (the common ancestor of the transcript's and the
// composer's Plate trees). PointerSensor, not native DnD, which Tauri's OS file-drop intercepts;
// the drop-target highlight is cleared on every end/cancel so it cannot outlive the drag.
export function DndScope({ children }: { children: ReactNode }) {
  const sensors = useSensors(useSensor(PointerSensor, POINTER_SENSOR_OPTIONS))
  const [target, setTarget] = useState<AttachmentDropTarget | null>(null)

  const onDragMove = useCallback((event: DragMoveEvent) => {
    const next = resolveAttachmentDrop(event)?.target ?? null
    setTarget((prev) => (sameTarget(prev, next) ? prev : next))
  }, [])
  const onDragEnd = useCallback((event: DragEndEvent) => {
    setTarget(null)
    const drop = resolveAttachmentDrop(event)
    if (drop) applyAttachmentDrop(drop.drag, drop.move)
  }, [])
  const onDragCancel = useCallback(() => setTarget(null), [])

  // `children` is a new element on every render of the chat view, which is every
  // streamed token. Keeping the dnd-kit subtree the same element and handing the
  // children down by context re-renders only them, in the same commit.
  const scope = useMemo(
    () => (
      <DndContext
        sensors={sensors}
        accessibility={ACCESSIBILITY}
        collisionDetection={pointerWithin}
        onDragMove={onDragMove}
        onDragOver={onDragMove}
        onDragEnd={onDragEnd}
        onDragCancel={onDragCancel}
      >
        <AttachmentDropTargetContext.Provider value={target}>
          <ScopeChildrenOutlet />
        </AttachmentDropTargetContext.Provider>
      </DndContext>
    ),
    [sensors, target, onDragMove, onDragEnd, onDragCancel],
  )
  return <ScopeChildren.Provider value={children}>{scope}</ScopeChildren.Provider>
}
