/**
 * selection-drag — "a pointer is down inside an editor, dragging out a
 * selection", as one fact both CSS and JS can read.
 *
 * The `data-editor-selecting` attribute on `<html>` already existed for the CSS
 * half (editor-theme.css promotes `.monaco-editor` to its own CALayer for the
 * span of the drag). This owns that attribute so the JS half — throttling the
 * per-selection-change cursor sync in `editor-surface.tsx` — reads the same
 * fact from a module flag instead of re-parsing the DOM on the hot path.
 *
 * Refcounted: a window can hold several editor panes, and a drag that starts in
 * one and releases over another must not leave the flag stuck on (or clear it
 * while a second pane is still dragging).
 */

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

/**
 * Drop this pane's claim on the drag without caring whether it had one — for
 * unmount cleanup, where a pane torn down mid-drag would otherwise pin the flag
 * (and the CALayer promotion) on forever.
 */
export function releaseSelectionDrag(held: boolean): void {
  if (held) endSelectionDrag()
}

/** Test-only: forget any in-flight drags. */
export function __resetSelectionDragForTests(): void {
  depth = 0
  if (typeof document !== 'undefined') document.documentElement.removeAttribute(ATTRIBUTE)
}
