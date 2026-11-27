import { EDITOR_CONSTANTS } from '../config/constants'

export function calculateLineHeight(
  fontSize: number,
  lineHeight: number = EDITOR_CONSTANTS.LINE_HEIGHT_MULTIPLIER,
): number {
  return Math.ceil(fontSize * lineHeight)
}
