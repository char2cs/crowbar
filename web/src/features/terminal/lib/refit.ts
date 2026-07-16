// Fit xterm to its container AND reconcile the daemon PTY to xterm's resulting
// grid — in one step, so no fit caller can leave the two out of sync.
//
// The bug this exists to close: fitAddon.fit() only makes xterm emit its
// onResize (the sole place that pushed the PTY size) when xterm's own rows/cols
// actually CHANGE. On an agent-chat pane split the terminal REMOUNTS at the new
// (halved) container height, but the daemon PTY keeps its old row count. If the
// fit is a no-op for xterm (grid already matches the container, or the callback
// was swallowed by the pane-resize guard) NO onResize fires and the PTY is never
// told it shrank — a full-screen TUI (alt buffer) keeps drawing at the bottom of
// its old, taller model, so the visible top half is blank.
//
// The fix is to gate the PTY push on "differs from what we last SYNCED TO THE
// DAEMON", not on "this fit changed xterm". That pushes the correct size even
// when fit() is a no-op (xterm already 28 rows, daemon still 56 → push 28) while
// still skipping redundant pushes (xterm 28, daemon already 28 → nothing), so it
// never spams resize IPC on a fit that changed nothing meaningful.
//
// Kept framework-free and DOM-light (structural param types, no xterm/DOM
// construction) so it is unit-testable with plain fakes.

/** The rows/cols last pushed to a specific daemon connection. */
export interface LastSyncedPtyDims {
  connId: string
  rows: number
  cols: number
}

/** Minimal shape of the container element this helper reads. */
interface RefitContainer {
  getBoundingClientRect: () => { width: number; height: number }
}

/** Minimal shape of the fit addon this helper drives. */
interface RefitFitAddon {
  fit: () => void
}

/** Minimal shape of the xterm terminal this helper reads. */
interface RefitTerminal {
  rows: number
  cols: number
}

export interface RefitAndSyncPtyArgs {
  container: RefitContainer | null
  fitAddon: RefitFitAddon | null
  terminal: RefitTerminal | null
  /** Current daemon connection id (or null/undefined when not yet attached). */
  getConnectionId: () => string | null | undefined
  /** Push the reconciled size to the daemon. */
  resize: (connectionId: string, rows: number, cols: number) => void
  /** Persists the last size pushed per connection so redundant fits don't re-push. */
  lastSyncedRef: { current: LastSyncedPtyDims | null }
}

/**
 * Fit xterm to its container, then reconcile the daemon PTY to xterm's current
 * grid. Returns without fitting or pushing when the container is 0×0 (hidden /
 * not yet laid out) — preserving the existing 0-area guard. The push is made
 * whenever xterm's grid differs from the last size synced to THIS connection
 * (including when the connection id changed), never merely when the fit itself
 * changed xterm.
 */
export function refitAndSyncPty({
  container,
  fitAddon,
  terminal,
  getConnectionId,
  resize,
  lastSyncedRef,
}: RefitAndSyncPtyArgs): void {
  if (!container || !fitAddon || !terminal) return

  const rect = container.getBoundingClientRect()
  // 0-area guard: fitting a hidden/unlaid-out container yields garbage dims and
  // a resize into that would corrupt the PTY. Do nothing until it has a box.
  if (rect.width <= 0 || rect.height <= 0) return

  fitAddon.fit()

  const connId = getConnectionId()
  if (!connId) return

  const { rows, cols } = terminal
  const last = lastSyncedRef.current

  // Already reconciled this exact grid to this connection — skip to avoid
  // resize-IPC spam on redundant fits. A different connId (reconnect / swap)
  // falls through and re-syncs, which is the desired "reset on id change".
  if (last && last.connId === connId && last.rows === rows && last.cols === cols) {
    return
  }

  resize(connId, rows, cols)
  lastSyncedRef.current = { connId, rows, cols }
}

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
