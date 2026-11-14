// "A pointer is dragging out a selection in an editor" as one fact: `data-editor-selecting` on
// <html> for the CSS (a CALayer promotion) and a module flag for editor-surface's cursor-sync
// throttle. Refcounted, so a drag across panes never leaves it stuck or clears it early.

const ATTRIBUTE = 'data-editor-selecting'

let depth = 0

/** True while at least one editor pane has a selection drag in flight. */
export function isSelectionDragging(): boolean {
  return depth > 0
}

/** Pointer went down in an editor: start (or join) a selection drag. */
export function beginSelectionDrag(): void {
  depth++
  if (depth === 1 && typeof document !== 'undefined') {
    document.documentElement.setAttribute(ATTRIBUTE, '1')
  }
}

/** Pointer released/cancelled. No-op when no drag is in flight. */
export function endSelectionDrag(): void {
  if (depth === 0) return
  depth--
  if (depth === 0 && typeof document !== 'undefined') {
    document.documentElement.removeAttribute(ATTRIBUTE)
  }
}

/** Drop this pane's claim on the drag if it had one, for unmount: a pane torn down mid-drag would
 *  otherwise pin the flag (and the CALayer promotion) forever. */
export function releaseSelectionDrag(held: boolean): void {
  if (held) endSelectionDrag()
}
