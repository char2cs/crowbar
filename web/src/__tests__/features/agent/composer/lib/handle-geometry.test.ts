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

  // Two lines are still a full pill with plus beside send; the rounded rectangle starts at three.
  it('stays a pill through two lines and wraps at the third', () => {
    expect(isMultiline(40)).toBe(false)
    expect(isMultiline(60)).toBe(true)
  })
})

describe('sendInset', () => {
  // THE TWO ARE ONE NUMBER: change the diameter or the padding and the button
  // stops being centred in the box it is nested in.
  it('insets the shipped 30px button by 5px on every side', () => {
    expect(sendInset()).toBe(5)
  })

  it('follows the diameter', () => {
    expect(sendInset(24)).toBe(8)
    expect(sendInset(SEND_DIAMETER, 20, 8)).toBe(3)
  })
})

describe('handleClusterWidth', () => {
  it('is one circle wide with a single occupant', () => {
    expect(handleClusterWidth(1)).toBe(SEND_DIAMETER)
  })

  it('adds one diameter and one gap for the second occupant', () => {
    expect(handleClusterWidth(2)).toBe(2 * SEND_DIAMETER + HANDLE_GAP)
  })

  it('the plus button matches the send button diameter', () => {
    expect(PLUS_DIAMETER).toBe(SEND_DIAMETER)
  })
})

describe('fieldRightPadding', () => {
  // The handle's reach from the padding edge; the field's `padding-right` is this less the
  // pill's side padding, plus the gap before the handle.
  it('reaches 67px in for two occupants', () => {
    expect(fieldRightPadding()).toBe(67)
  })

  it('shrinks to the single-occupant reservation', () => {
    expect(fieldRightPadding(sendInset(), 1)).toBe(35)
  })

  it('follows HANDLE_GAP', () => {
    expect(fieldRightPadding(4, 2, 28, 3)).toBe(4 + 28 + 3 + 28)
  })
})

// The phone's capsule: 20px line, 16px padding, 34pt circles (hit-slop makes them 44).
describe('the phone capsule', () => {
  // composer-pill.css touch `.field` padding-right and `.handle` right inset.
  it('insets its 34pt circles by 9px and reaches 87px in for them, 10px apart', () => {
    expect(sendInset(34, 20, 16)).toBe(9)
    expect(fieldRightPadding(9, 2, 34, 10)).toBe(87)
  })
})
