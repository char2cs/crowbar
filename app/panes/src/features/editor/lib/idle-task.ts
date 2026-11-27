export interface IdleTaskHandle {
  cancel: () => void
}

export interface ScheduleIdleOptions {
  timeout?: number
}

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
