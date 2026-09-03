import { describe, expect, it } from 'vitest'

import {
  COMPOSER_LINE_HEIGHT,
  handleOffset,
  isMultiline,
  sendInset,
  SEND_DIAMETER,
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

  it('is true once a second line lands', () => {
    expect(isMultiline(40)).toBe(true)
  })
})

describe('sendInset', () => {
  // THE TWO ARE ONE NUMBER: change the diameter or the padding and the button
  // stops being centred in the box it is nested in.
  it('insets the shipped button by 4px on every side', () => {
    expect(sendInset()).toBe(4)
  })

  it('follows the diameter', () => {
    expect(sendInset(24)).toBe(6)
    expect(sendInset(SEND_DIAMETER, 20, 10)).toBe(6)
  })
})
