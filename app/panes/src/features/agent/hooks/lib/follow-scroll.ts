export interface FollowScroll {
  setTarget(target: number): void
  stop(): void
  pause(): void
}

const TAU_MS = 100
const SETTLE_PX = 0.5

const MAX_FRAME_MS = 32

const MIN_STEP_PX = 0.05

export function createFollowScroll(
  el: HTMLElement,
  onTick?: (value: number) => void,
): FollowScroll {
  let target: number | null = null
  let lastWritten = el.scrollTop
  let lastTime = 0
  let raf = 0
  let stopped = false

  const step = (now: number) => {
    raf = 0
    if (stopped || target === null) return
    if (Math.abs(el.scrollTop - lastWritten) > 1) return

    const dt = Math.min(now - lastTime, MAX_FRAME_MS)
    lastTime = now
    const from = el.scrollTop
    const distance = target - from
    let landed = Math.abs(distance) <= SETTLE_PX
    el.scrollTop = landed ? target : from + distance * (1 - Math.exp(-dt / TAU_MS))
    lastWritten = el.scrollTop
    if (!landed && Math.abs(lastWritten - from) < MIN_STEP_PX) {
      el.scrollTop = target
      lastWritten = el.scrollTop
      landed = true
    }
    onTick?.(lastWritten)
    if (!landed) raf = requestAnimationFrame(step)
  }

  return {
    setTarget(next) {
      if (stopped) return
      target = next
      lastWritten = el.scrollTop
      if (raf === 0) {
        lastTime = performance.now()
        raf = requestAnimationFrame(step)
      }
    },
    stop() {
      stopped = true
      this.pause()
    },
    pause() {
      target = null
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    },
  }
}
