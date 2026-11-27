export interface RafCoalescer {
  schedule: () => void
  flush: () => void
  cancel: () => void
}

export interface RafCoalescerOptions {
  minIntervalMs?: () => number
}

const hasRaf = () => typeof requestAnimationFrame === 'function'
const now = () =>
  typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now()

export function createRafCoalescer(
  task: () => void,
  options: RafCoalescerOptions = {},
): RafCoalescer {
  let handle: number | null = null
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
