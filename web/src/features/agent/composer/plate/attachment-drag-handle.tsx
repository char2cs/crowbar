import { createContext, useCallback, useContext, useId } from 'react'
import { DotsSixVerticalIcon, TrashIcon } from '@phosphor-icons/react'
import {
  type DragMoveEvent,
  type DraggableAttributes,
  type UniqueIdentifier,
  useDraggable,
  useDroppable,
} from '@dnd-kit/core'
import { type Path, PathApi, type TElement } from 'platejs'
import { type PlateEditor, useComposedRef, useEditorRef } from 'platejs/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** `outline`'s other states are partial-alpha, which shows a photo through the
 *  button. `--popover` is the only solid neutral token, hence no hover shift. */
export const ATTACHMENT_BUTTON_OPAQUE_BG = cn(
  'bg-popover dark:bg-popover',
  'hover:bg-popover dark:hover:bg-popover',
  'data-pressed:bg-popover dark:data-pressed:bg-popover',
)

export type DropLine = 'top' | 'bottom'

export interface AttachmentDndData {
  editor: PlateEditor
  element: TElement
}

/** Owned by `DndScope` (dnd-scope.tsx), which recomputes it on every drag move. */
export interface AttachmentDropTarget {
  id: UniqueIdentifier
  line: DropLine
}

export const AttachmentDropTargetContext = createContext<AttachmentDropTarget | null>(null)

export interface AttachmentMove {
  at: Path
  to: Path
}

/** Attachments reorder only among their own siblings at the SAME level — not
 *  into a list item or a table cell. */
export function canDropAttachmentNode(dragPath: Path, dropPath: Path): boolean {
  return PathApi.equals(PathApi.parent(dragPath), PathApi.parent(dropPath))
}

/** `null` when the drop is not allowed or would leave the block where it is. */
export function attachmentDropMove(
  drag: AttachmentDndData,
  drop: AttachmentDndData,
  line: DropLine,
): AttachmentMove | null {
  if (drag.editor !== drop.editor || drag.element === drop.element) return null
  const dragPath = drag.editor.api.findPath(drag.element)
  const hoveredPath = drop.editor.api.findPath(drop.element)
  if (!dragPath || !hoveredPath || !canDropAttachmentNode(dragPath, hoveredPath)) return null
  let dropPath: Path
  if (line === 'bottom') {
    dropPath = hoveredPath
    if (PathApi.equals(dragPath, PathApi.next(dropPath))) return null
  } else {
    dropPath = [...hoveredPath.slice(0, -1), hoveredPath.at(-1)! - 1]
    if (PathApi.equals(dragPath, dropPath)) return null
  }
  const to =
    PathApi.isBefore(dragPath, dropPath) && PathApi.isSibling(dragPath, dropPath)
      ? dropPath
      : PathApi.next(dropPath)
  return { at: dragPath, to }
}

type DragEventLike = Pick<DragMoveEvent, 'active' | 'over' | 'activatorEvent' | 'delta'>

function pointerY({ activatorEvent, delta }: DragEventLike): number | undefined {
  const start = activatorEvent as Partial<PointerEvent> | null
  return typeof start?.clientY === 'number' ? start.clientY + delta.y : undefined
}

export function resolveAttachmentDrop(
  event: DragEventLike,
): { target: AttachmentDropTarget; drag: AttachmentDndData; move: AttachmentMove } | null {
  const { active, over } = event
  const drag = active.data.current as AttachmentDndData | undefined
  const drop = over?.data.current as AttachmentDndData | undefined
  const y = pointerY(event)
  if (!over || !drag?.editor || !drop?.editor || y === undefined) return null
  const line: DropLine = y < over.rect.top + over.rect.height / 2 ? 'top' : 'bottom'
  const move = attachmentDropMove(drag, drop, line)
  return move && { target: { id: over.id, line }, drag, move }
}

export function applyAttachmentDrop({ editor, element }: AttachmentDndData, move: AttachmentMove) {
  editor.tf.select(element)
  editor.tf.moveNodes(move)
}

function useDropLine(id: UniqueIdentifier): DropLine | undefined {
  const target = useContext(AttachmentDropTargetContext)
  return target?.id === id ? target.line : undefined
}

/** A file card's link sits inside its own paragraph; `canDropAttachmentNode`
 *  compares parents, so the nested link must resolve to that block. */
export function resolveDraggableBlockElement(editor: PlateEditor, element: TElement): TElement {
  const path = editor.api.findPath(element)
  if (!path) return element
  const ancestor = editor.api.above<TElement>({
    at: path,
    match: (n) => editor.api.isBlock(n),
    mode: 'highest',
  })
  return ancestor ? ancestor[0] : element
}

export type AttachmentHandleProps = {
  ref: (node: HTMLElement | null) => void
} & Partial<DraggableAttributes> &
  Partial<Record<string, unknown>>

/** Outside a `DndScope` dnd-kit's hooks are inert, so a message renders the same
 *  with or without one. */
export function useAttachmentDraggable(element: TElement) {
  const editor = useEditorRef()
  const blockElement = resolveDraggableBlockElement(editor, element)
  const id = useId()
  const data: AttachmentDndData = { editor, element: blockElement }
  const draggable = useDraggable({ id, data })
  const droppable = useDroppable({ id, data })
  const nodeRef = useComposedRef<HTMLElement>(draggable.setNodeRef, droppable.setNodeRef)
  const handleProps: AttachmentHandleProps = {
    ref: draggable.setActivatorNodeRef,
    ...draggable.attributes,
    ...draggable.listeners,
  }
  // The resolved block, not a file card's nested link.
  const remove = useCallback(() => {
    const path = editor.api.findPath(blockElement)
    if (path) editor.tf.removeNodes({ at: path })
  }, [editor, blockElement])
  return {
    isDragging: draggable.isDragging,
    nodeRef,
    handleProps,
    dropLine: useDropLine(id),
    remove,
  }
}

/** Droppable, never draggable, so an attachment can land anywhere in the message. */
export function useAttachmentDropTarget(element: TElement) {
  const editor = useEditorRef()
  const id = useId()
  const data: AttachmentDndData = { editor, element }
  const { setNodeRef } = useDroppable({ id, data })
  return { nodeRef: setNodeRef, dropLine: useDropLine(id) }
}

/** `contentEditable={false}`: inside Slate's editable region WebKit treats
 *  pointerdown+move as a text selection before the drag can start. */
export function AttachmentDragHandle({
  handleProps,
  onSelect,
}: {
  handleProps?: AttachmentHandleProps
  onSelect?: () => void
}) {
  return (
    <div contentEditable={false} className="contents">
      <Button
        {...handleProps}
        variant="outline"
        size="icon-xs"
        aria-label="Reorder this attachment"
        className={cn(
          ATTACHMENT_BUTTON_OPAQUE_BG,
          'focus-visible:ring-0 focus-visible:ring-offset-0',
          'cursor-grab touch-none active:cursor-grabbing',
        )}
        onClick={onSelect}
      >
        <DotsSixVerticalIcon className="text-muted-foreground" />
      </Button>
    </div>
  )
}

export function AttachmentDeleteButton({ onDelete }: { onDelete: () => void }) {
  return (
    <Button
      variant="outline"
      size="icon-xs"
      aria-label="Remove this attachment"
      className={cn(
        ATTACHMENT_BUTTON_OPAQUE_BG,
        'focus-visible:ring-0 focus-visible:ring-offset-0',
      )}
      onClick={onDelete}
    >
      <TrashIcon className="text-muted-foreground" />
    </Button>
  )
}

/** Inset inside the top edge, not straddling it: a first attachment has no top
 *  margin, so anything above the box is clipped by `.field`'s overflow. */
export function AttachmentControls({
  handleProps,
  onDelete,
}: {
  handleProps?: AttachmentHandleProps
  onDelete: () => void
}) {
  return (
    <div
      contentEditable={false}
      className="-translate-x-1/2 absolute top-1 left-1/2 z-51 flex flex-row items-center gap-1 opacity-0 transition-opacity duration-100 group-hover/attachment:opacity-100"
    >
      <AttachmentDragHandle handleProps={handleProps} />
      <AttachmentDeleteButton onDelete={onDelete} />
    </div>
  )
}

export function AttachmentDropLine({ line }: { line: DropLine | undefined }) {
  if (!line) return null
  return (
    <div
      className={cn(
        'absolute inset-x-0 left-2 z-50 h-0.5 bg-brand/50',
        line === 'top' ? '-top-px' : '-bottom-px',
      )}
    />
  )
}
