import type { TText } from 'platejs'
import {
  CHAT_FRESH_BORN_MARK,
  CHAT_FRESH_DELAY_MARK,
  CHAT_FRESH_MARK,
} from '@/features/agent/transcript/plate/streaming-value-patch'

// How long `chat-token-fade` itself runs — keep in step with transcript.css.
const FADE_DURATION_MS = 260

// One frame, the resolution `resumedFadeDelay` measures a run's age in.
const FRAME_MS = 16

/** The fade delay a DECORATED leaf (`props.leaf`) carries, if any. */
function freshLeafDelay(leaf: TText): number | null {
  const record = leaf as unknown as Record<string, unknown>
  if (typeof record[CHAT_FRESH_MARK] !== 'number') return null
  const delay = record[CHAT_FRESH_DELAY_MARK]
  return typeof delay === 'number' ? delay : 0
}

// First paint per run, keyed by `bornAt`: one chunk's words resolve in a single
// commit that may straddle a frame, and a shared origin keeps their cascade.
const firstPaintedAt = new Map<number, number>()
// Only has to stop unbounded growth across a long session.
const MAX_TRACKED_PAINTS = 512

function paintOriginFor(bornAt: number, now: number): number {
  const seen = firstPaintedAt.get(bornAt)
  if (seen !== undefined) return seen
  if (firstPaintedAt.size >= MAX_TRACKED_PAINTS) {
    // Insertion-ordered, so the oldest half goes first.
    let drop = MAX_TRACKED_PAINTS / 2
    for (const key of firstPaintedAt.keys()) {
      if (drop-- <= 0) break
      firstPaintedAt.delete(key)
    }
  }
  firstPaintedAt.set(bornAt, now)
  return now
}

/** The `animation-delay` to mount with: the word's wait minus time already on
 *  screen, so a remount resumes rather than restarts (negative = started that
 *  long ago). Call once per mount; clamped at the fade's length. */
export function resumedFadeDelay(leaf: TText): number | null {
  const base = freshLeafDelay(leaf)
  if (base === null) return null
  const bornAt = (leaf as unknown as Record<string, unknown>)[CHAT_FRESH_BORN_MARK]
  if (typeof bornAt !== 'number') return base
  const now = typeof performance === 'object' ? performance.now() : Date.now()
  // Floored to whole frames so a first-frame mount gets its nominal delay exactly.
  const elapsed = Math.max(0, Math.floor((now - paintOriginFor(bornAt, now)) / FRAME_MS) * FRAME_MS)
  return Math.max(base - elapsed, -FADE_DURATION_MS)
}
