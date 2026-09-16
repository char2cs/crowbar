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

/** The plus button's diameter — the SAME circle as send, so the two read as
 *  one control family rather than two mismatched controls glued together. */
export const PLUS_DIAMETER = SEND_DIAMETER

/** `.handle`'s own flex `gap`, between however many buttons it holds. */
export const HANDLE_GAP = 2

/**
 * The button cluster's total width, edge to edge, for however many circles
 * `.handle` holds (2 today: plus + send).
 *
 * THIS NUMBER AND THE FIELD'S OWN `padding-right` ARE ONE NUMBER, same rule
 * as `sendInset`: the field reserves exactly `sendInset() + this` so wrapped
 * text never renders under a button. Add a third control without widening
 * the field to match and text starts running underneath it.
 */
export function handleClusterWidth(
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return occupants * diameter + Math.max(0, occupants - 1) * gap
}

/**
 * The field's own right padding, derived rather than picked.
 *
 * At the shipped values (4px inset, two 28px circles, one 2px gap) that is
 * 4 + 28 + 2 + 28 = 62 — exactly what `.agent-chat .pill .field` already
 * ships as `padding-right`, with zero slack to spare. That is not
 * coincidence papered over after the fact: the field was already sized for
 * a second control before this one existed. It is also why adding the plus
 * button requires zeroing `.handle .send`'s own `margin-left` in
 * composer.css — keeping BOTH that margin and `.handle`'s flex `gap` would
 * double-count the space between the two buttons and blow through this
 * budget by exactly `SEND_MARGIN_LEFT`'s old value (5px).
 */
export function fieldRightPadding(
  rightInset: number = sendInset(),
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return rightInset + handleClusterWidth(occupants, diameter, gap)
}
