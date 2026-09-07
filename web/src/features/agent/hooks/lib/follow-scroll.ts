export interface FollowScroll {
  /** Arms (or re-targets) the loop to keep converging on `target`. Safe to
   *  call every frame, including while already running toward an earlier
   *  target — that just changes what the NEXT tick converges toward, rather
   *  than restarting the approach. */
  setTarget(target: number): void
  /** Permanently stops the loop — for unmount. A stop from a real scroll
   *  gesture interrupting mid-flight is NOT this: it only pauses the loop
   *  (see `step`), and a later `setTarget` re-arms it cleanly. */
  stop(): void
}

// How quickly `scrollTop` closes the remaining distance to its target, as a
// time constant: after TAU_MS it has covered ~63% of what was left, after
// 3x that ~95% — "mostly there" reads at roughly 3x this value.
const TAU_MS = 100
const SETTLE_PX = 0.5

// The longest gap this will smooth over in ONE frame, however long the main
// thread was actually blocked.
//
// Exponential smoothing over real elapsed time is right in continuous time
// and wrong on a screen: a frame arriving 250ms late "should" close 1 -
// e^(-250/100) = 92% of the remaining distance, but the frames in between
// were never painted, so nobody sees a fast glide — they see the transcript
// teleport. Reported live as "scroll bouncing, not stable", and only ever
// while the fps HUD was in the red, which is the tell: at a steady 60fps this
// value never binds at all. Capping the step makes a recovering scroll read
// as catching up instead of jumping; the target keeps moving to the bottom
// anyway, so it still converges, just over a few more frames.
const MAX_FRAME_MS = 32

// Below this, a write did not move the element at all — it is already at its
// own scrollable edge and the target is simply out of reach (content shrank
// under us). Stopping is right; spinning a rAF loop that rewrites the same
// pixel forever is not.
const MIN_STEP_PX = 0.05

/**
 * Continuously eases `el.scrollTop` toward a target that can itself keep
 * moving — a growing transcript retargets this every time a new line
 * appears, often faster than any one ease could finish.
 *
 * This exists because a fixed-duration tween, restarted on every retarget,
 * is the wrong tool for that: each restart resets to the STEEP part of its
 * own ease-out curve, and a target that moves faster than the tween's
 * duration means every restart gets cut short a few pixels in — which reads
 * as choppy, near-instant motion, not one smooth glide. Recomputing
 * `distance = target - el.scrollTop` fresh every frame, with no notion of
 * "restart", is what makes retargeting mid-flight free: the loop just keeps
 * converging on wherever the target currently is.
 *
 * Self-interrupting, the same way a fixed-duration tween would: if
 * `el.scrollTop` differs from what this loop itself wrote last frame,
 * something else (a real scroll gesture) moved it, and the loop pauses
 * rather than fighting it. `setTarget` re-arms cleanly from wherever
 * `scrollTop` actually is when that happens.
 */
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
    // Something else wrote scrollTop since our last frame — a real gesture.
    // Pause; `setTarget` (called again once the caller decides to resume
    // following) re-syncs `lastWritten` and restarts cleanly.
    if (Math.abs(el.scrollTop - lastWritten) > 1) return

    // Capped, not the raw elapsed time — see MAX_FRAME_MS.
    const dt = Math.min(now - lastTime, MAX_FRAME_MS)
    lastTime = now
    const from = el.scrollTop
    const distance = target - from
    if (Math.abs(distance) <= SETTLE_PX) {
      el.scrollTop = target
      // Read back here too: even the final write is clamped if the target is
      // past the container's edge.
      lastWritten = el.scrollTop
      onTick?.(lastWritten)
      return
    }
    const closed = 1 - Math.exp(-dt / TAU_MS)
    el.scrollTop = from + distance * closed
    // READ BACK what the container actually took. It clamps to its own
    // scrollable ceiling, and that ceiling drops whenever the content shrinks
    // under us (a turn releasing the room reserved to pin it to the top does
    // exactly that). `lastWritten` is not bookkeeping for this loop alone —
    // it is also what `use-transcript-anchor` hands to `onTick` and compares
    // the next scroll EVENT against, to tell its own writes apart from the
    // reader's gesture. Reporting a position the element never took made the
    // very next scroll event look like the reader grabbing the scrollbar, so
    // the transcript decided it was no longer stuck and quietly stopped
    // following the reply for the rest of the turn.
    lastWritten = el.scrollTop
    onTick?.(lastWritten)
    if (Math.abs(lastWritten - from) < MIN_STEP_PX) return
    raf = requestAnimationFrame(step)
  }

  return {
    setTarget(next) {
      if (stopped) return
      target = next
      lastWritten = el.scrollTop
      if (raf === 0) {
        // Captured NOW, not deferred to the first callback: rAF's own
        // scheduling delay then becomes that first frame's real `dt`,
        // instead of a frame that measures zero elapsed time and moves
        // nothing.
        lastTime = performance.now()
        raf = requestAnimationFrame(step)
      }
    },
    stop() {
      stopped = true
      target = null
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    },
  }
}
