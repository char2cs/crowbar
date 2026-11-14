// Collapse a burst of synchronous calls into one trailing requestAnimationFrame flush (cursor and
// selection store sync off the keystroke path). Trailing, so it reads final state; `cancel()` on
// dispose so a queued frame never hits a torn-down editor. Falls back to setTimeout(fn, 0).

export interface RafCoalescer {
  /** Schedule a trailing flush of `task` for the next frame (idempotent). */
  schedule: () => void
  /** Run a pending task now (synchronously) and clear the scheduled frame. */
  flush: () => void
  /** Drop a pending task without running it. Call on dispose. */
  cancel: () => void
}

export interface RafCoalescerOptions {
  /** Read at schedule time: while > 0, flush at most once per that many ms instead of per frame,
   *  for bursts that outlive a gesture. Still trailing; call `flush()` when the gesture ends so
   *  the final state isn't left in a throttled timer. */
  minIntervalMs?: () => number
}

const hasRaf = () => typeof requestAnimationFrame === 'function'
const now = () =>
  typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now()

/** Wrap `task` in a once-per-frame trailing scheduler: `schedule()` calls within a frame collapse
 *  to one invocation that reads the latest state. */
export function createRafCoalescer(
  task: () => void,
  options: RafCoalescerOptions = {},
): RafCoalescer {
  let handle: number | null = null
  // Whether the pending handle was created via rAF (vs the setTimeout fallback),
  // so it is cancelled with the matching API even if the env changes between
  // schedule and cancel.
  let usedRaf = false
  let lastRun = Number.NEGATIVE_INFINITY

  const clear = () => {
    if (handle === null) return
    if (usedRaf) cancelAnimationFrame(handle)
    else clearTimeout(handle as unknown as ReturnType<typeof setTimeout>)
    handle = null
  }

  const run = () => {
    handle = null
    lastRun = now()
    task()
  }

  return {
    schedule: () => {
      if (handle !== null) return
      const minInterval = options.minIntervalMs?.() ?? 0
      const wait = minInterval > 0 ? minInterval - (now() - lastRun) : 0
      if (wait > 0) {
        usedRaf = false
        handle = setTimeout(run, wait) as unknown as number
        return
      }
      usedRaf = hasRaf()
      handle = usedRaf ? requestAnimationFrame(run) : (setTimeout(run, 0) as unknown as number)
    },
    flush: () => {
      if (handle === null) return
      clear()
      run()
    },
    cancel: clear,
  }
}
