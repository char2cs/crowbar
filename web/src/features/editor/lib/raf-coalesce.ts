/**
 * raf-coalesce — collapse a burst of synchronous calls into ONE trailing
 * `requestAnimationFrame` flush.
 *
 * Used to take the per-cursor / per-keystroke cursor+selection store sync OFF
 * the editor hot path: every cursor move / model-content change calls
 * `schedule()`, but the wrapped task runs at most ONCE per animation frame —
 * reading the editor's CURRENT state once and writing it coalesced. A burst of N
 * keystrokes therefore produces ≤1 store write per frame instead of N.
 *
 * The task is trailing (it reads live state at flush time, so it always reflects
 * the final position of the burst). `flush()` runs any pending task synchronously
 * (e.g. before a deliberate read of the store); `cancel()` drops a pending task
 * (call on dispose so a queued frame never fires against a torn-down editor).
 *
 * Falls back to `setTimeout(fn, 0)` where `requestAnimationFrame` is absent
 * (jsdom, background tabs) so behavior is preserved off the rAF path.
 */

export interface RafCoalescer {
  /** Schedule a trailing flush of `task` for the next frame (idempotent). */
  schedule: () => void
  /** Run a pending task now (synchronously) and clear the scheduled frame. */
  flush: () => void
  /** Drop a pending task without running it. Call on dispose. */
  cancel: () => void
}

const hasRaf = () => typeof requestAnimationFrame === 'function'

/**
 * Wrap `task` in a once-per-frame trailing scheduler. `task` reads live state at
 * flush time, so repeated `schedule()` calls within a frame collapse to a single
 * invocation that reflects the latest state.
 */
export function createRafCoalescer(task: () => void): RafCoalescer {
  let handle: number | null = null
  // Whether the pending handle was created via rAF (vs the setTimeout fallback),
  // so it is cancelled with the matching API even if the env changes between
  // schedule and cancel.
  let usedRaf = false

  const clear = () => {
    if (handle === null) return
    if (usedRaf) cancelAnimationFrame(handle)
    else clearTimeout(handle as unknown as ReturnType<typeof setTimeout>)
    handle = null
  }

  const run = () => {
    handle = null
    task()
  }

  return {
    schedule: () => {
      if (handle !== null) return
      usedRaf = hasRaf()
      handle = usedRaf ? requestAnimationFrame(run) : (setTimeout(run, 0) as unknown as number)
    },
    flush: () => {
      if (handle === null) return
      clear()
      task()
    },
    cancel: clear,
  }
}
