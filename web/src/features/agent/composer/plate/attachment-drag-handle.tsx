import { GripVertical } from 'lucide-react'
import { type CanDropCallback, type DragItemNode, useDraggable, useDropLine } from '@platejs/dnd'
import { PathApi, type TElement } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * `BlockMenuKit` (components/editor/plugins/block-menu-kit.tsx) does NOT
 * carry a drag-to-reorder handle in this codebase — its own upstream
 * template comment names a `dnd-kit.tsx` (block-selection-kit.tsx:6) that
 * was never actually added here; `BlockMenuKit` is block SELECTION plus a
 * right-click context menu only. The only real drag-reorder primitive that
 * exists in this repo is `@platejs/dnd`'s `useDraggable`/`useDropLine`,
 * already used for table-row reordering (components/ui/table-node.tsx:
 * 1087-1187) — this module is that same primitive, scoped to a single
 * attachment block instead of a table row, with none of `BlockMenuKit`'s
 * would-be `/`-insert or block-type-conversion chrome (chat deliberately
 * has neither).
 */

// Exported standalone so both can be unit-tested directly, without the
// HTML5 drag/pointer-capture wiring `useDraggable` layers on top.
export const canDropAttachmentNode: CanDropCallback = ({ dragEntry, dropEntry }) =>
  PathApi.equals(PathApi.parent(dragEntry[1]), PathApi.parent(dropEntry[1]))

export function onAttachmentDropHandler(
  editor: PlateEditor,
  { dragItem }: { dragItem: DragItemNode },
) {
  const dragElement = (dragItem as { element?: TElement }).element
  if (dragElement) editor.tf.select(dragElement)
}

export function useAttachmentDraggable(element: TElement) {
  return useDraggable({
    element,
    type: element.type,
    // Attachments reorder only among their own siblings at the SAME level —
    // not into a list item or a table cell, mirroring the table row's own
    // same-parent constraint.
    canDropNode: canDropAttachmentNode,
    onDropHandler: onAttachmentDropHandler,
  })
}

export function AttachmentDragHandle({
  dragRef,
  onSelect,
}: {
  dragRef: React.Ref<HTMLButtonElement> | null
  onSelect?: () => void
}) {
  return (
    <Button
      ref={dragRef ?? undefined}
      variant="outline"
      aria-label="Reorder this attachment"
      className={cn(
        '-translate-y-1/2 absolute top-1/2 left-0 z-51 h-6 w-4 p-0 focus-visible:ring-0 focus-visible:ring-offset-0',
        'cursor-grab active:cursor-grabbing',
        'opacity-0 transition-opacity duration-100 group-hover/attachment:opacity-100',
      )}
      onClick={onSelect}
    >
      <GripVertical className="text-muted-foreground" />
    </Button>
  )
}

export function AttachmentDropLine() {
  const { dropLine } = useDropLine()
  if (!dropLine) return null
  return (
    <div
      className={cn(
        'absolute inset-x-0 left-2 z-50 h-0.5 bg-brand/50',
        dropLine === 'top' ? '-top-px' : '-bottom-px',
      )}
    />
  )
}
