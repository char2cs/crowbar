import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { createFollowScroll, type FollowScroll } from '@/features/agent/hooks/lib/follow-scroll'

/* How close to the bottom still counts as reading the newest end.
   Roughly one message row. Generous on purpose: a turn that lands several
   blocks at once can outrun a single scroll frame, and a reader who is
   obviously at the bottom should not stop being followed because the content
   grew faster than the scroll did. */
const STICK_SLACK = 96

export interface TranscriptAnchor {
  /** Goes on the scroll container. Its first element child is the content. */
  scrollRef: React.RefObject<HTMLDivElement | null>
  /** Goes on the same container's onScroll. */
  onScroll: () => void
  /** Call immediately BEFORE prepending older messages. */
  preservePosition: () => void
}

/**
 * Keeps a transcript pinned to its newest message.
 *
 * The anchoring is measured, not declarative, because the two things a chat does
 * — grow at the bottom as a turn streams, grow at the TOP when older messages
 * page in — need opposite responses, and CSS cannot tell them apart.
 *
 * Growth is observed rather than derived from a revision prop: a turn changes
 * height for reasons no counter sees (a tool row resolving, a subagent shelf
 * appearing, prose reflowing on a pane resize), and every one of them must keep
 * the newest line on screen.
 *
 * Following stops the moment the reader scrolls up. That is the whole contract:
 * a chat that yanks you back to the bottom while you are reading history is
 * worse than one that never follows at all.
 */
export function useTranscriptAnchor(): TranscriptAnchor {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const stuck = useRef(true)
  // Distance from the bottom to restore after a prepend, or null when the next
  // resize is ordinary growth.
  const restoreFromBottom = useRef<number | null>(null)
  const follow = useRef<FollowScroll | null>(null)
  // The value `follow`'s own writes are producing, right now — how `onScroll`
  // tells "that's just this loop ticking" apart from a real scroll gesture
  // landing mid-flight, since both change the same property. Null whenever
  // nothing is currently following.
  const expectedScrollTop = useRef<number | null>(null)

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [])

  useEffect(() => {
    const el = scrollRef.current
    const content = el?.firstElementChild
    if (!el || !content) return
    follow.current = createFollowScroll(el, (v) => {
      expectedScrollTop.current = v
    })
    const observer = new ResizeObserver(() => {
      const keep = restoreFromBottom.current
      if (keep !== null) {
        // Older messages just landed above the fold. Holding the distance from
        // the BOTTOM — not scrollTop — is what leaves the row the reader was
        // looking at exactly where it was. Instant, deliberately: nothing here
        // is "the newest line arriving", so easing it would read as the whole
        // transcript sliding for no visible reason.
        restoreFromBottom.current = null
        el.scrollTop = el.scrollHeight - keep
        return
      }
      // The scrollable ceiling, not the raw content height — el.scrollTop can
      // never legally exceed scrollHeight - clientHeight. Feeding the raw
      // scrollHeight in here doesn't just settle at the wrong place: the
      // exponential-smoothing math below computes its per-frame step from
      // this value BEFORE the browser ever gets a chance to clamp anything,
      // so an error the size of clientHeight (typically far bigger than one
      // chunk's growth) makes the very first frame overshoot the REAL
      // ceiling — which the native scrollTop setter then clamps to
      // instantly. The result is indistinguishable from no easing at all.
      if (stuck.current) follow.current?.setTarget(el.scrollHeight - el.clientHeight)
    })
    // BOTH, and the second one is the half that was missing. The conversation
    // slides out of view for two different reasons: the content grows (a turn
    // streams) or the VIEWPORT shrinks (the composer grows a line under it).
    // Watching only the content meant typing a second line quietly pushed the
    // newest message behind the box — nothing had resized, so nothing followed.
    observer.observe(content)
    observer.observe(el)
    return () => {
      observer.disconnect()
      follow.current?.stop()
      follow.current = null
    }
  }, [])

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    // A scroll event the follow loop's OWN writes produced echoes back a
    // value it just wrote — matching within a pixel is what tells that apart
    // from the reader's own gesture, and it must not count as "scrolled up":
    // the loop runs precisely BECAUSE the reader is still at the bottom.
    if (
      expectedScrollTop.current !== null &&
      Math.abs(el.scrollTop - expectedScrollTop.current) <= 1
    ) {
      return
    }
    // Real input: the reader's own scroll wins immediately, even mid-glide —
    // the loop's own drift check would catch this too on its next frame, but
    // clearing the expected value here means the NEXT tick doesn't have to.
    expectedScrollTop.current = null
    // Re-armed as soon as the reader comes back to the bottom, so following
    // resumes without them having to do anything but scroll down.
    stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_SLACK
  }, [])

  const preservePosition = useCallback(() => {
    const el = scrollRef.current
    if (el) restoreFromBottom.current = el.scrollHeight - el.scrollTop
  }, [])

  return { scrollRef, onScroll, preservePosition }
}
