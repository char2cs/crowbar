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

/**
 * Forces a genuinely OPAQUE background on `variant="outline"`'s Button, in
 * every state it defines one for — reported live, three separate times, as
 * "still transparent" until this: `outline`'s own classes (button-variants.ts)
 * are `bg-popover` at rest but `dark:bg-input/32`/`hover:bg-accent/50`/
 * `dark:hover:bg-input/64`/`data-pressed:bg-input/64` everywhere else — ALL
 * partial-alpha mixes. That reads fine for a button sitting on a UNIFORM
 * surface (which is every other place this app uses those tokens), but these
 * buttons sit on top of an attachment's own content — a photo, a busy
 * preview — which is never uniform, so any alpha background shows whatever
 * is underneath right through it. `--popover`/`--card` (styles/theme.css)
 * are the only neutral surface tokens in this theme that resolve to a real
 * solid color (`var(--background)` in dark mode, `var(--color-white)` in
 * light) rather than a `color-mix(..., transparent)` blend — every state
 * gets pinned to that SAME solid tone, deliberately with no hover color
 * shift, since there is no other solid neutral token in this theme to shift
 * to without reintroducing the same transparency.
 */
export const ATTACHMENT_BUTTON_OPAQUE_BG = cn(
  'bg-popover dark:bg-popover',
  'hover:bg-popover dark:hover:bg-popover',
  'data-pressed:bg-popover dark:data-pressed:bg-popover',
)

export type DropLine = 'top' | 'bottom'

/** What every attachment draggable and drop target registers with dnd-kit:
 *  the editor it lives in and the top-level block it stands for. */
export interface AttachmentDndData {
  editor: PlateEditor
  element: TElement
}

/** The drop target under the pointer, and which edge of it the dragged
 *  attachment would land on. Owned by `DndScope` (dnd-scope.tsx), which
 *  recomputes it on every drag move and clears it when the drag ends. */
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

/**
 * Where dropping `drag` on the `line` edge of `drop` moves the dragged block —
 * or `null` when the drop is not allowed (another editor, another parent, the
 * block itself) or would leave the block exactly where it already is.
 */
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

/** The pointer's current y: where the drag started plus how far it moved. */
function pointerY({ activatorEvent, delta }: DragEventLike): number | undefined {
  const start = activatorEvent as Partial<PointerEvent> | null
  return typeof start?.clientY === 'number' ? start.clientY + delta.y : undefined
}

/**
 * Resolves a dnd-kit drag event to the drop target to highlight and the move
 * to apply on release. The line is the half of the hovered block the pointer
 * is in.
 */
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

/** Applies a resolved drop: selects the dragged block, then moves it. */
export function applyAttachmentDrop({ editor, element }: AttachmentDndData, move: AttachmentMove) {
  editor.tf.select(element)
  editor.tf.moveNodes(move)
}

function useDropLine(id: UniqueIdentifier): DropLine | undefined {
  const target = useContext(AttachmentDropTargetContext)
  return target?.id === id ? target.line : undefined
}

/**
 * The top-level block ancestor of `element` — itself, if `element` already
 * IS one (a fence-based attachment like a code block), otherwise the highest
 * block ancestor above it (a file card's wrapping paragraph:
 * `insertAttachmentMarkdown` inserts each `[filename](ref)` as its own
 * single-child paragraph, so the link itself sits one level below the top).
 * `canDropAttachmentNode` compares parents, so an unresolved nested link
 * could never match a sibling block.
 */
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

/** What the drag handle spreads onto its button: dnd-kit's activator ref, its
 *  a11y attributes and its pointer listeners. */
export type AttachmentHandleProps = {
  ref: (node: HTMLElement | null) => void
} & Partial<DraggableAttributes> &
  Partial<Record<string, unknown>>

/**
 * An attachment block's drag-and-drop: draggable from its handle, and a drop
 * target itself so attachments reorder against each other. Outside a
 * `DndScope` dnd-kit's hooks are inert — no context, no throw — so a message
 * renders the same with or without one.
 */
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
  // The SAME resolved block dragging operates on — a file card's `element`
  // is its nested link, not the paragraph that's actually reorderable/
  // removable (see `resolveDraggableBlockElement`).
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

/**
 * A plain paragraph's half of drag-and-drop: droppable, never draggable, so an
 * attachment can land anywhere in the message, not just next to another
 * attachment.
 */
export function useAttachmentDropTarget(element: TElement) {
  const editor = useEditorRef()
  const id = useId()
  const data: AttachmentDndData = { editor, element }
  const { setNodeRef } = useDroppable({ id, data })
  return { nodeRef: setNodeRef, dropLine: useDropLine(id) }
}

/**
 * `contentEditable={false}` here is load-bearing, not cosmetic: without it
 * this button sits inside the Slate editor's own `contenteditable="true"`
 * region, and WebKit (Tauri's WKWebView) arbitrates a real pointerdown+move
 * there as a text-selection gesture before the drag can start.
 */
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

/** The drag handle and delete button as one floating pair, centered on the
 *  attachment's own x-axis, inset just inside its top edge. Not straddling
 *  the border: a first/only attachment's own top margin is unconditionally
 *  zeroed (`[data-slate-node='element']` in composer.css), so anything
 *  poking above the box gets clipped by `.field`'s `overflow-y: auto`. */
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
