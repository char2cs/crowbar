import { describe, expect, it } from 'vitest'

import {
  COMPOSER_LINE_HEIGHT,
  handleOffset,
  isMultiline,
  sendInset,
  SEND_DIAMETER,
  handleClusterWidth,
  fieldRightPadding,
  HANDLE_GAP,
  PLUS_DIAMETER,
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

describe('handleClusterWidth', () => {
  it('is one circle wide with a single occupant', () => {
    expect(handleClusterWidth(1)).toBe(SEND_DIAMETER)
  })

  it('adds one diameter and one gap for the second occupant', () => {
    expect(handleClusterWidth(2)).toBe(58)
  })

  it('the plus button matches the send button diameter', () => {
    expect(PLUS_DIAMETER).toBe(SEND_DIAMETER)
  })
})

describe('fieldRightPadding', () => {
  // THE TWO ARE ONE NUMBER, same as sendInset: this must equal the literal
  // `padding-right` composer.css already ships on `.pill .field`.
  it('matches the shipped field padding-right exactly, for two occupants', () => {
    expect(fieldRightPadding()).toBe(62)
  })

  it('shrinks to the single-occupant reservation', () => {
    expect(fieldRightPadding(sendInset(), 1)).toBe(32)
  })

  it('follows HANDLE_GAP', () => {
    expect(fieldRightPadding(4, 2, 28, 3)).toBe(4 + 28 + 3 + 28)
  })
})
