export const COMPOSER_LINE_HEIGHT = 20

/** Rides the last line, not the caret's: the bottom is where a hand goes for send. */
export function handleOffset(fieldHeight: number): number {
  return Math.max(0, Math.round(fieldHeight - COMPOSER_LINE_HEIGHT))
}

/** A second line turns the pill into a rounded rectangle. 6px of slack: fractional font-metric
 *  heights make a bare `> LINE` flicker. */
export function isMultiline(fieldHeight: number): boolean {
  return fieldHeight > COMPOSER_LINE_HEIGHT + 6
}
