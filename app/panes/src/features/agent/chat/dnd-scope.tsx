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

const ACCESSIBILITY = {
  screenReaderInstructions: { draggable: 'Drag the handle to move this attachment.' },
}

const POINTER_SENSOR_OPTIONS = { activationConstraint: { distance: 4 } }

function sameTarget(a: AttachmentDropTarget | null, b: AttachmentDropTarget | null) {
  return a?.id === b?.id && a?.line === b?.line
}

const ScopeChildren = createContext<ReactNode>(null)

function ScopeChildrenOutlet() {
  return useContext(ScopeChildren)
}

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
