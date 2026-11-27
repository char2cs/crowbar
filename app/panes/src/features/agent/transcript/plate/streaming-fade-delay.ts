import type { TText } from 'platejs'
import {
  CHAT_FRESH_BORN_MARK,
  CHAT_FRESH_DELAY_MARK,
  CHAT_FRESH_MARK,
} from '@/features/agent/transcript/plate/streaming-value-patch'

const FADE_DURATION_MS = 260

const FRAME_MS = 16

function freshLeafDelay(leaf: TText): number | null {
  const record = leaf as unknown as Record<string, unknown>
  if (typeof record[CHAT_FRESH_MARK] !== 'number') return null
  const delay = record[CHAT_FRESH_DELAY_MARK]
  return typeof delay === 'number' ? delay : 0
}

const firstPaintedAt = new Map<number, number>()
const MAX_TRACKED_PAINTS = 512

function paintOriginFor(bornAt: number, now: number): number {
  const seen = firstPaintedAt.get(bornAt)
  if (seen !== undefined) return seen
  if (firstPaintedAt.size >= MAX_TRACKED_PAINTS) {
    let drop = MAX_TRACKED_PAINTS / 2
    for (const key of firstPaintedAt.keys()) {
      if (drop-- <= 0) break
      firstPaintedAt.delete(key)
    }
  }
  firstPaintedAt.set(bornAt, now)
  return now
}

export function resumedFadeDelay(leaf: TText): number | null {
  const base = freshLeafDelay(leaf)
  if (base === null) return null
  const bornAt = (leaf as unknown as Record<string, unknown>)[CHAT_FRESH_BORN_MARK]
  if (typeof bornAt !== 'number') return base
  const now = typeof performance === 'object' ? performance.now() : Date.now()
  const elapsed = Math.max(0, Math.floor((now - paintOriginFor(bornAt, now)) / FRAME_MS) * FRAME_MS)
  return Math.max(base - elapsed, -FADE_DURATION_MS)
}
