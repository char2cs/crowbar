import type { Range } from '@tanstack/react-virtual'
import type { TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** `.stream`'s 18px gap, baked into each virtual row so the offsets count it. */
export const ROW_GAP = 18

/** Markdown by its blocks: a paragraph wraps by the column (7.5px a glyph), a fence is its lines. */
function textHeight(text: string, column: number): number {
  const perLine = Math.floor(column / 7.5)
  let height = 0
  let fenced = false
  for (const line of text.split('\n')) {
    const fence = line.startsWith('```')
    fenced = fenced !== fence
    if (fence) height += 12
    else if (fenced) height += 19
    else height += line ? Math.ceil(line.length / perLine) * 23 : 14
  }
  return height
}

/** By text and box, so a first guess is near the height: 26px around a reply, 35 a prompt. */
export function estimateRowHeight(id: string, item: TranscriptItem | undefined, box = 768): number {
  if (id.startsWith('g:') || item?.kind === 'tool') return 30 // a call, or a run of them, is one line: a card's height overstates the list ~3x
  const column = Math.min(box, 768) - 44
  if (item?.kind === 'user') return 35 + textHeight(item.text, column * 0.8)
  if (item?.kind === 'text') return 26 + textHeight(item.text, column)
  return 96
}

/** Pixels of rows kept drawn past each edge of the view: a fling covers this in a frame or two,
 *  which a row-count overscan does not for one-line rows. */
const REACH_PX = 800

/** The window as the virtualizer measured it, widened by `REACH_PX` at both ends. */
export const reachRange =
  (sizeOf: (index: number) => number, overscan: number) =>
  (range: Range): number[] => {
    const last = range.count - 1
    const reached = (from: number, step: 1 | -1) => {
      let i = from
      for (let reach = 0; i + step >= 0 && i + step <= last && reach < REACH_PX;) {
        i += step
        reach += sizeOf(i)
      }
      return i
    }
    const first = Math.min(Math.max(range.startIndex - overscan, 0), reached(range.startIndex, -1))
    const end = Math.max(Math.min(range.endIndex + overscan, last), reached(range.endIndex, 1))
    return Array.from({ length: end - first + 1 }, (_, k) => first + k)
  }
