/** One text line in the composer, in px. Everything else is derived from it. */
export const COMPOSER_LINE_HEIGHT = 20

/** The pill's vertical padding, per side. */
export const COMPOSER_PADDING_Y = 8

/** The send button's diameter. */
export const SEND_DIAMETER = 28

/**
 * The handle's offset from the top of the field.
 *
 * It rides the LAST line, not the caret's: on one line it sits inline on the
 * right, and once the box grows it slides to the bottom and stays there — which
 * is where a hand goes for send.
 */
export function handleOffset(fieldHeight: number): number {
  return Math.max(0, Math.round(fieldHeight - COMPOSER_LINE_HEIGHT))
}

/**
 * Has the field wrapped past one line?
 *
 * Measured with 6px of slack, because a field reports fractional heights from
 * font metrics and a bare `> LINE` flickers the radius on a single line.
 */
export function isMultiline(fieldHeight: number): boolean {
  return fieldHeight > COMPOSER_LINE_HEIGHT + 6
}

/**
 * The send button's inset from every edge of the pill.
 *
 * THE TWO ARE ONE NUMBER. The circle overhangs the text line by the pill's
 * vertical padding, and `right` must match that overhang or the button sits
 * visibly off-centre in a box it is supposed to be nested in:
 *
 *     inset = (lineHeight + 2 * paddingY - diameter) / 2
 *
 * At the shipped values that is (20 + 16 - 28) / 2 = 4.
 */
export function sendInset(
  diameter: number = SEND_DIAMETER,
  lineHeight: number = COMPOSER_LINE_HEIGHT,
  paddingY: number = COMPOSER_PADDING_Y,
): number {
  return (lineHeight + 2 * paddingY - diameter) / 2
}
