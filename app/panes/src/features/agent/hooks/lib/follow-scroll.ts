export interface FollowScroll {
  /** Arms or re-targets the loop; safe every frame, mid-flight it only moves the goal. */
  setTarget(target: number): void
  /** Permanently stops the loop, for unmount. A gesture interrupting mid-flight only pauses it
   *  (see `step`). */
  stop(): void
  /** Drops the target: the reader took over. A later `setTarget` re-arms it. */
  pause(): void
}

// Time constant of the ease: after TAU_MS ~63% of the remaining distance is closed, after 3x ~95%.
const TAU_MS = 100
const SETTLE_PX = 0.5

// The longest gap one frame smooths over: a late frame would otherwise teleport the transcript.
const MAX_FRAME_MS = 32

// A write that moved less than this hit the scrollable edge: stop rather than rewrite one pixel forever.
const MIN_STEP_PX = 0.05

// Eases scrollTop toward a target that moves as the transcript grows. Not a restarted tween, whose
// every restart re-enters its steep start and stutters: the distance is recomputed each frame.
// A scrollTop this loop did not write is a gesture, and pauses it.
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
    // Something else wrote scrollTop since our last frame: a gesture. Pause; `setTarget` re-syncs.
    if (Math.abs(el.scrollTop - lastWritten) > 1) return

    // Capped, not the raw elapsed time — see MAX_FRAME_MS.
    const dt = Math.min(now - lastTime, MAX_FRAME_MS)
    lastTime = now
    const from = el.scrollTop
    const distance = target - from
    let landed = Math.abs(distance) <= SETTLE_PX
    el.scrollTop = landed ? target : from + distance * (1 - Math.exp(-dt / TAU_MS))
    // Read back what the container took: it clamps when content shrinks, and `onTick` hands this
    // to use-transcript-anchor to tell our writes from a gesture.
    lastWritten = el.scrollTop
    if (!landed && Math.abs(lastWritten - from) < MIN_STEP_PX) {
      // An engine that keeps whole pixels drops a sub-pixel step: write the last pixels at once.
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
        // Captured now so rAF's scheduling delay becomes the first frame's `dt`, rather than a
        // first frame measuring zero elapsed time and moving nothing.
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
