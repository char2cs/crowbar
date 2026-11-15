import { describe, expect, it } from 'vitest'

import {
  COMPOSER_LINE_HEIGHT,
  handleOffset,
  isMultiline,
} from '@/features/agent/composer/lib/handle-geometry'

describe('handleOffset', () => {
  it('sits inline on one line', () => {
    expect(handleOffset(COMPOSER_LINE_HEIGHT)).toBe(0)
  })

  it('rides the LAST line as the box grows', () => {
    expect(handleOffset(40)).toBe(20)
    expect(handleOffset(60)).toBe(40)
    expect(handleOffset(80)).toBe(60)
  })

  it('never goes negative on a sub-line measurement', () => {
    expect(handleOffset(0)).toBe(0)
    expect(handleOffset(12)).toBe(0)
  })
})

describe('isMultiline', () => {
  // Fractional font metrics make a bare `> LINE` flicker the radius on one line.
  it('tolerates fractional height on a single line', () => {
    expect(isMultiline(20)).toBe(false)
    expect(isMultiline(24.5)).toBe(false)
  })

  // The rounded rectangle starts the moment the text wraps, so two lines already read as a box.
  it('turns into the rounded rectangle as soon as a second line appears', () => {
    expect(isMultiline(40)).toBe(true)
    expect(isMultiline(60)).toBe(true)
  })
})
