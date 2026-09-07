import { useCallback, useContext, useRef } from 'react'
import { GripVertical, Trash2Icon } from 'lucide-react'
import { DndContext } from 'react-dnd'
import {
  type CanDropCallback,
  DndPlugin,
  type DragItemNode,
  useDraggable,
  useDropLine,
  useDropNode,
} from '@platejs/dnd'
import { PathApi, type TElement } from 'platejs'
import { type PlateEditor, useEditorRef } from 'platejs/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * ONE dnd type for every attachment kind, instead of each `useDraggable`
 * call using `blockElement.type` (a code-block attachment's `'code_block'`
 * vs. a file card's wrapping `'p'`) — react-dnd only lets a drop target
 * accept drags of the SAME type it was told to `accept`, so two attachments
 * of different underlying Slate node types could never be reordered against
 * each other, and a plain paragraph (also `'p'`, coincidentally, but never
 * registered as a drop target at all) had no way to accept one either.
 * Sharing one constant, unrelated to the Slate schema type, is what lets
 * `useAttachmentDropTarget` below register ordinary paragraphs as valid drop
 * targets alongside attachment blocks — the whole point being "move an
 * attachment to any position in the message, not just swap it with another
 * attachment."
 */
export const ATTACHMENT_DND_TYPE = 'chat-attachment'

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

/**
 * The top-level block ancestor of `element` — itself, if `element` already
 * IS one (a fence-based attachment like a code block, whose own `PlateElement`
 * root is a plain top-level block already), otherwise the highest matching
 * block ancestor above it (a file card's wrapping paragraph: `insertAttachment
 * Markdown` inserts each `[filename](ref)` as its own single-child paragraph,
 * so the link itself sits one level below the top).
 *
 * `canDropAttachmentNode` below compares `PathApi.parent(...)` on whatever
 * entries `@platejs/dnd` derives from the `element` handed to `useDraggable` —
 * for an inline anchor nested in a paragraph that comparison is between two
 * DIFFERENT paragraphs' paths and can never match. Resolving to the block
 * ancestor here, once, fixes that for every attachment kind without
 * `canDropAttachmentNode` itself needing to know about nesting at all.
 *
 * Exported standalone (same reasoning as `canDropAttachmentNode` above) so
 * this walk can be unit-tested directly against a real editor instance
 * without mounting `useDraggable`'s own HTML5 drag/pointer-capture wiring.
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

export function useAttachmentDraggable(element: TElement) {
  const editor = useEditorRef()
  const blockElement = resolveDraggableBlockElement(editor, element)
  const draggable = useDraggable({
    element: blockElement,
    type: ATTACHMENT_DND_TYPE,
    // Attachments reorder only among their own siblings at the SAME level —
    // not into a list item or a table cell, mirroring the table row's own
    // same-parent constraint.
    canDropNode: canDropAttachmentNode,
    onDropHandler: onAttachmentDropHandler,
    drag: {
      // `DndPlugin`'s own `handlers.onDragEnd` (@platejs/dnd) is the thing
      // that normally resets `dropTarget` — but it's wired to the native DOM
      // `dragend` event, which `TouchBackend` (dnd-scope.tsx) never fires,
      // since it drives dragging off plain mouse events instead of the
      // native Drag and Drop API. Left unhandled, the blue drop-line
      // indicator stays stuck showing after every drag. `end` here IS
      // backend-agnostic (dnd-core calls it whenever a drag ends, regardless
      // of which backend produced it), so the reset moves here instead —
      // same three resets `@platejs/dnd`'s own default `end` already does,
      // plus the `dropTarget` clear it was missing.
      end: () => {
        editor.setOption(DndPlugin, 'isDragging', false)
        editor.setOption(DndPlugin, 'dropTarget', { id: null, line: '' })
        document.body.classList.remove('dragging')
      },
    },
  })
  // The SAME resolved block dragging operates on — a file card's `element`
  // is its nested link, not the paragraph that's actually reorderable/
  // removable (see `resolveDraggableBlockElement`'s own doc comment).
  const remove = useCallback(() => {
    const path = editor.api.findPath(blockElement)
    if (path) editor.tf.removeNodes({ at: path })
  }, [editor, blockElement])
  return { ...draggable, remove }
}

/**
 * A plain paragraph's half of drag-and-drop: droppable, never draggable
 * itself. `useDraggable` bundles a drag SOURCE and a drop TARGET together
 * (via `useDndNode`'s internal `useDragNode` + `useDropNode` pair) — right
 * for an attachment, wrong for ordinary text, which must never itself become
 * something you can pick up and drag. `useDropNode` (the same primitive
 * `useDraggable` calls internally) used alone registers just the target half,
 * so a message's plain paragraphs can accept an attachment being dropped
 * between them without becoming draggable themselves.
 *
 * Registered on every interactive paragraph (`ChatParagraphElement` below) —
 * this, together with `ATTACHMENT_DND_TYPE` unifying every attachment kind
 * onto one dnd type, is what lets an attachment land anywhere in the
 * message, not just swap places with another attachment.
 *
 * Unlike `useAttachmentDraggable`, this has no `editor.plugins.dnd` guard to
 * lean on — `useDropNode` calls react-dnd's own `useDrop` unconditionally
 * (no such short-circuit inside `@platejs/dnd` itself), which THROWS
 * "Expected drag drop context" with no `<DndProvider>` ancestor. Every
 * message has plain paragraphs, so unconditionally registering this on all
 * of them would have made a `<DndProvider>` mandatory for rendering ANY
 * message at all, attachments or not — a much bigger requirement than this
 * feature needs. Peeking at `DndContext` directly (the same context
 * `useDragDropManager` reads, minus its `invariant`) and skipping
 * registration when it's absent keeps a plain paragraph renderable with no
 * `<DndProvider>`, exactly like before this hook existed.
 */
export function useAttachmentDropTarget(element: TElement) {
  const editor = useEditorRef()
  const nodeRef = useRef<HTMLElement | null>(null)
  const { dragDropManager } = useContext(DndContext)
  if (!dragDropManager) return { nodeRef }
  // Safe despite the shape react-hooks/rules-of-hooks flags in general: a
  // `<DndContext>` ancestor's PRESENCE (unlike its value) cannot change
  // across this component's own lifetime without remounting it — the same
  // invariant `@platejs/dnd`'s own `useDraggable` relies on for its
  // analogous `if (!editor.plugins.dnd) return {}` guard.
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const [, drop] = useDropNode(editor, {
    accept: [ATTACHMENT_DND_TYPE],
    canDropNode: canDropAttachmentNode,
    element,
    // No drag preview to show — a paragraph is a drop target only, never
    // draggable itself — but `UseDropNodeOptions` requires the field anyway.
    multiplePreviewRef: null,
    nodeRef,
    onDropHandler: onAttachmentDropHandler,
  })
  drop(nodeRef)
  return { nodeRef }
}

/**
 * `contentEditable={false}` here is load-bearing, not cosmetic: without it
 * this button sits inside the Slate editor's own `contenteditable="true"`
 * region, and WebKit (Tauri's WKWebView) arbitrates a real mousedown+move
 * there as a text-selection gesture BEFORE react-dnd's native `dragstart`
 * ever fires — regardless of the button's own `draggable="true"`, confirmed
 * live via DOM inspection (`-webkit-user-drag: element` was already correct;
 * the missing non-editable boundary was the actual blocker). Same escape
 * hatch the preview `<div>` right next to this handle already uses.
 */
export function AttachmentDragHandle({
  dragRef,
  onSelect,
}: {
  dragRef: React.Ref<HTMLButtonElement> | null
  onSelect?: () => void
}) {
  return (
    <div contentEditable={false} className="contents">
      <Button
        ref={dragRef ?? undefined}
        variant="outline"
        size="icon-xs"
        aria-label="Reorder this attachment"
        className={cn(
          ATTACHMENT_BUTTON_OPAQUE_BG,
          'focus-visible:ring-0 focus-visible:ring-offset-0',
          'cursor-grab active:cursor-grabbing',
        )}
        onClick={onSelect}
      >
        <GripVertical className="text-muted-foreground" />
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
      className={cn(ATTACHMENT_BUTTON_OPAQUE_BG, 'focus-visible:ring-0 focus-visible:ring-offset-0')}
      onClick={onDelete}
    >
      <Trash2Icon className="text-muted-foreground" />
    </Button>
  )
}

/** The drag handle and delete button as one floating pair, centered on the
 *  attachment's own x-axis, inset just inside its top edge. Not straddling
 *  the border: a first/only attachment's own top margin is unconditionally
 *  zeroed (`[data-slate-node='element']` in composer.css), so anything
 *  poking above the box gets clipped by `.field`'s `overflow-y: auto`. */
export function AttachmentControls({
  dragRef,
  onDelete,
}: {
  dragRef: React.Ref<HTMLButtonElement> | null
  onDelete: () => void
}) {
  return (
    <div
      contentEditable={false}
      className="-translate-x-1/2 absolute top-1 left-1/2 z-51 flex flex-row items-center gap-1 opacity-0 transition-opacity duration-100 group-hover/attachment:opacity-100"
    >
      <AttachmentDragHandle dragRef={dragRef} />
      <AttachmentDeleteButton onDelete={onDelete} />
    </div>
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
