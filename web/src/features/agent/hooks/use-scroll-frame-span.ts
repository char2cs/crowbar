import { useCallback, useRef } from 'react'
import { markEnd, markStart } from '@/lib/perf/instrumentation'

/** How long after the last scroll event a frame-sampling run stops. */
const SCROLL_IDLE_MS = 200

/**
 * Samples one `chat.scroll.frame` measure per animation frame while the
 * transcript is actively scrolling (spec §1: frame times, not CPU percentages
 * — a stalled frame and a busy one look identical any other way).
 *
 * Each frame's span opens the instant the previous one's `markEnd` runs, so
 * its duration IS that frame's cost. `markEnd` always runs before the
 * sampling-stopped check, so a frame in flight when scrolling goes idle is
 * still closed — no dangling `:start` mark survives past this hook's own
 * loop.
 */
export function useScrollFrameSpan(): { onScrollEvent: () => void } {
  const sampling = useRef(false)
  const idleTimer = useRef<ReturnType<typeof setTimeout>>()

  const sampleFrame = useCallback(() => {
    markEnd('chat.scroll.frame')
    if (!sampling.current) return
    markStart('chat.scroll.frame')
    requestAnimationFrame(sampleFrame)
  }, [])

  const onScrollEvent = useCallback(() => {
    if (!sampling.current) {
      sampling.current = true
      markStart('chat.scroll.frame')
      requestAnimationFrame(sampleFrame)
    }
    clearTimeout(idleTimer.current)
    idleTimer.current = setTimeout(() => {
      sampling.current = false
    }, SCROLL_IDLE_MS)
  }, [sampleFrame])

  return { onScrollEvent }
}
