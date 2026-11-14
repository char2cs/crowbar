// Run a one-shot callback off the critical frame, so deferred work (LSP open, first diagnostics,
// code lenses) lets content paint first. Cancelling before it fires guarantees it never runs,
// which keeps didOpen/didClose safe under rapid tab switching. Falls back to setTimeout(fn, 0).

export interface IdleTaskHandle {
  /** Cancel the pending task. Idempotent. No-op once the task has fired. */
  cancel: () => void
}

export interface ScheduleIdleOptions {
  /** Max delay (ms) before the idle callback is forced to run. Default 200. */
  timeout?: number
}

/** Schedule `task` for browser idle (or `timeout` ms at the latest); `cancel()` stops it if it
 *  hasn't fired. */
export function scheduleIdleTask(
  task: () => void,
  options: ScheduleIdleOptions = {},
): IdleTaskHandle {
  const timeout = options.timeout ?? 200
  let cancelled = false

  if (typeof requestIdleCallback === 'function') {
    const id = requestIdleCallback(
      () => {
        if (cancelled) return
        task()
      },
      { timeout },
    )
    return {
      cancel: () => {
        if (cancelled) return
        cancelled = true
        cancelIdleCallback(id)
      },
    }
  }

  const id = setTimeout(() => {
    if (cancelled) return
    task()
  }, 0)
  return {
    cancel: () => {
      if (cancelled) return
      cancelled = true
      clearTimeout(id)
    },
  }
}
