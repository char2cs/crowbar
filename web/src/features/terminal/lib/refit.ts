// Drag-end recovery for the terminal's fit: see pollUntilResizeSettles.

export interface ResizeSettlePollDeps {
  /** True while a pane/sidebar drag is in progress (data-pane-resizing set). */
  isResizing: () => boolean
  /** Invoked exactly once, after isResizing() first returns false. */
  onSettled: () => void
  /** Animation-frame scheduler (injected so tests can drive it deterministically). */
  requestFrame: (cb: () => void) => number
  /** Animation-frame canceller. */
  cancelFrame: (id: number) => void
}

/**
 * Poll on animation frames until `isResizing()` returns false, then call
 * `onSettled` exactly once. This is the drag-end recovery that does NOT depend
 * on the one-shot `pane-resize-end` event: a terminal remounted mid-drag (a pane
 * split) never receives that event, so relying on it alone drops the final fit
 * and leaves the daemon PTY at its pre-resize size — the blank-top bug. Fitting
 * is deferred while the drag is live (per-frame fit + resize IPC is expensive);
 * this guarantees the last container size is applied once the drag ends.
 *
 * Returns a canceller (for effect cleanup / an earlier pane-resize-end) that
 * stops the poll and guarantees `onSettled` will not fire afterwards.
 */
export function pollUntilResizeSettles(deps: ResizeSettlePollDeps): () => void {
  let frame: number | null = null
  let cancelled = false

  const tick = () => {
    frame = null
    if (cancelled) return
    if (deps.isResizing()) {
      frame = deps.requestFrame(tick)
      return
    }
    deps.onSettled()
  }

  frame = deps.requestFrame(tick)

  return () => {
    cancelled = true
    if (frame !== null) {
      deps.cancelFrame(frame)
      frame = null
    }
  }
}
