import { describe, expect, it } from 'vitest'
import { dropLineBox } from '@/components/layout/drop-indicator'

const row = (top: number, left: number) => ({ top, bottom: top + 36, left, width: 240 })

describe('the line', () => {
  it('sits on the top edge of the target row for a "before"', () => {
    expect(dropLineBox(row(100, 0), 'before').top).toBe(99)
  })

  it('sits on its bottom edge for an "after"', () => {
    expect(dropLineBox(row(100, 0), 'after').top).toBe(135)
  })

  it('insets into the row, leaving room for the end-cap', () => {
    expect(dropLineBox(row(100, 0), 'before')).toEqual({ left: 8, top: 99, width: 230 })
  })

  it('inherits the depth from the row it marks', () => {
    expect(dropLineBox(row(100, 28), 'before').left).toBe(36)
  })

  it('never goes negative on a row narrower than its own insets', () => {
    expect(dropLineBox({ top: 0, bottom: 36, left: 0, width: 4 }, 'before').width).toBe(0)
  })
})
