import { useCallback } from 'react'

/** How long something has run, the one way the chat writes it: `58s`, `1m 42s`, `1h 05m`. */
export function formatClock(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  if (safe < 60) return `${safe}s`
  const pad = (n: number) => String(n).padStart(2, '0')
  if (safe < 3600) return `${Math.floor(safe / 60)}m ${pad(safe % 60)}s`
  return `${Math.floor(safe / 3600)}h ${pad(Math.floor((safe % 3600) / 60))}m`
}

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
  format: (seconds: number) => string = formatClock,
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
