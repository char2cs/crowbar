import { useCallback } from 'react'
import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'

const clocks = new Map<HTMLElement, { since: number; format: (seconds: number) => string }>()
let timer: number | null = null

function write(el: HTMLElement, clock: { since: number; format: (seconds: number) => string }) {
  el.textContent = clock.format(Math.max(0, Math.round((Date.now() - clock.since) / 1000)))
}

const tick = () => clocks.forEach((clock, el) => write(el, clock))

/** Writes the time since `since` into `el` every second, with no React render;
 *  one interval serves every clock on the page. Returns the clock's stop. */
function startClock(
  el: HTMLElement,
  since: number,
  format: (seconds: number) => string = formatElapsed,
): () => void {
  const clock = { since, format }
  clocks.set(el, clock)
  write(el, clock)
  timer ??= window.setInterval(tick, 1000)
  return () => {
    clocks.delete(el)
    if (clocks.size > 0 || timer === null) return
    window.clearInterval(timer)
    timer = null
  }
}

/** A ref that runs a clock from `since` (an ISO time) while its element is mounted. */
export function useTextClock(since: string | undefined, format?: (seconds: number) => string) {
  return useCallback(
    (el: HTMLElement | null) => {
      const at = since ? Date.parse(since) : Number.NaN
      if (!el || Number.isNaN(at)) return undefined
      return startClock(el, at, format)
    },
    [since, format],
  )
}
