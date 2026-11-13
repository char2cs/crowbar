export const COMPOSER_LINE_HEIGHT = 20

/** The pill's vertical padding, per side. */
export const COMPOSER_PADDING_Y = 10

export const SEND_DIAMETER = 30

/** Rides the last line, not the caret's: the bottom is where a hand goes for send. */
export function handleOffset(fieldHeight: number): number {
  return Math.max(0, Math.round(fieldHeight - COMPOSER_LINE_HEIGHT))
}

/** A third line turns the pill into a rounded rectangle; two still read as a pill. 6px of slack:
 *  fractional font-metric heights make a bare `> 2 * LINE` flicker. */
export function isMultiline(fieldHeight: number): boolean {
  return fieldHeight > 2 * COMPOSER_LINE_HEIGHT + 6
}

/** One inset for every edge: `right` must match the vertical overhang or the
 *  button sits off-centre in the pill. */
export function sendInset(
  diameter: number = SEND_DIAMETER,
  lineHeight: number = COMPOSER_LINE_HEIGHT,
  paddingY: number = COMPOSER_PADDING_Y,
): number {
  return (lineHeight + 2 * paddingY - diameter) / 2
}

/** The same circle as send, so the two read as one control family. */
export const PLUS_DIAMETER = SEND_DIAMETER

export const HANDLE_GAP = 2

/** The field reserves exactly `sendInset() + this`, so a third control must widen
 *  the field or wrapped text runs underneath it. */
export function handleClusterWidth(
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return occupants * diameter + Math.max(0, occupants - 1) * gap
}

/** How far in from the pill's padding edge the handle reaches: the field's `padding-right`
 *  (57px) is this less the pill's 18px side padding, plus the 8px between text and handle. No
 *  `margin-left` on `.handle .send` on top of the flex gap, or the two would double-count. */
export function fieldRightPadding(
  rightInset: number = sendInset(),
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return rightInset + handleClusterWidth(occupants, diameter, gap)
}
