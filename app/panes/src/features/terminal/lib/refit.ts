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

/** Calls `onSettled` once `isResizing()` is false. A terminal remounted mid-drag never gets the
 *  one-shot `pane-resize-end`, so this guarantees the deferred final fit. The canceller ensures
 *  `onSettled` never fires afterwards. */
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

/** There is no DPR event: a resolution media query fires once per flip, so it is re-armed at the
 *  new ratio. */
export function watchDevicePixelRatio(onChange: () => void): () => void {
  let mql: MediaQueryList | null = null
  const onFlip = () => {
    onChange()
    arm()
  }
  const arm = () => {
    mql?.removeEventListener('change', onFlip)
    mql = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
    mql.addEventListener('change', onFlip)
  }
  arm()
  return () => mql?.removeEventListener('change', onFlip)
}
