import type { Range } from '@tanstack/react-virtual'
import type { TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** `.stream`'s 18px gap, baked into each virtual row so the offsets count it. */
export const ROW_GAP = 18

const ESTIMATED_ROW_HEIGHT = 96
/** A call, or a run of them, is one line: estimating it as a card overstates the list ~3x. */
const ESTIMATED_CALL_HEIGHT = 30
const ROW_CHROME = 8 + 30 + ROW_GAP
const ROW_LINE_HEIGHT = 23
const CHARS_PER_LINE = 88

/** Scaled by the text so a long reply's first guess is close to its height. */
export function estimateRowHeight(id: string, item: TranscriptItem | undefined): number {
  if (id.startsWith('g:') || item?.kind === 'tool') return ESTIMATED_CALL_HEIGHT
  if (item?.kind !== 'text' && item?.kind !== 'user') return ESTIMATED_ROW_HEIGHT
  const lines = Math.max(1, Math.ceil(item.text.length / CHARS_PER_LINE))
  return Math.max(ESTIMATED_ROW_HEIGHT, ROW_CHROME + lines * ROW_LINE_HEIGHT)
}

/** Pixels of rows kept drawn past each edge of the view: a fling covers this in a frame or two,
 *  which a row-count overscan does not for one-line rows. */
const REACH_PX = 800

/** The window as the virtualizer measured it, widened by `REACH_PX` at both ends. */
export const reachRange =
  (sizeOf: (index: number) => number, overscan: number) =>
  (range: Range): number[] => {
    let first = Math.max(range.startIndex - overscan, 0)
    for (let reach = 0, i = range.startIndex; i > 0 && reach < REACH_PX; i--) {
      reach += sizeOf(i - 1)
      first = Math.min(first, i - 1)
    }
    let last = Math.min(range.endIndex + overscan, range.count - 1)
    for (let reach = 0, i = range.endIndex; i < range.count - 1 && reach < REACH_PX; i++) {
      reach += sizeOf(i + 1)
      last = Math.max(last, i + 1)
    }
    return Array.from({ length: last - first + 1 }, (_, k) => first + k)
  }
