export const COMPOSER_LINE_HEIGHT = 20

export function handleOffset(fieldHeight: number): number {
  return Math.max(0, Math.round(fieldHeight - COMPOSER_LINE_HEIGHT))
}

export function isMultiline(fieldHeight: number): boolean {
  return fieldHeight > COMPOSER_LINE_HEIGHT + 6
}
