/**
 * idle-task — schedule a one-shot callback off the critical frame.
 *
 * Used to defer expensive, non-paint-blocking work (LSP `documentOpen`, initial
 * diagnostics marker paint, code-lens fetch) a beat AFTER a buffer is shown so
 * the content paints immediately. Returns a cancel handle; cancelling before the
 * callback fires guarantees it never runs — critical for LSP `didOpen`/`didClose`
 * lifecycle safety under rapid tab switching.
 *
 * Prefers `requestIdleCallback` (with a `timeout` so it can't starve), falling
 * back to `setTimeout(fn, 0)` where the API is absent (jsdom, WKWebView).
 */

export interface IdleTaskHandle {
  /** Cancel the pending task. Idempotent. No-op once the task has fired. */
  cancel: () => void
}

export interface ScheduleIdleOptions {
  /** Max delay (ms) before the idle callback is forced to run. Default 200. */
  timeout?: number
}

/**
 * Schedule `task` to run when the browser is idle (or after `timeout` ms at the
 * latest). Returns a handle whose `cancel()` prevents the task from firing if it
 * hasn't already.
 */
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
