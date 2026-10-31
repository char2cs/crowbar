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

/**
 * The drag-and-drop scope for attachment reordering, one per chat view.
 * `AgentChatView` is the common ancestor of every Plate tree that renders an
 * attachment node live: the transcript's streaming `MarkdownMessage` and the
 * composer's `ChatMarkdownEditor`.
 *
 * dnd-kit's `PointerSensor` drives the drag from ordinary pointer events, never
 * the native Drag and Drop API — which Tauri's own OS-file-drop interception
 * (`dragDropEnabled`, see tauri-file-drop.ts) swallows before it reaches the
 * DOM. External file drops keep going through Tauri's `onDragDropEvent`,
 * unrelated to this in-page reordering. The 4px activation distance keeps a
 * click on the handle a click.
 *
 * This component owns the one piece of drag state — the highlighted drop
 * target — and clears it on every end or cancel, so no drop line can outlive
 * the drag that drew it.
 */
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
