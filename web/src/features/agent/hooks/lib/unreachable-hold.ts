/** A held distance from the bottom that is larger than the whole content points
 *  above the first row: the reader's place is not in what is loaded now. */
export function isUnreachableHold(scrollHeight: number, distanceFromBottom: number | null) {
  return distanceFromBottom !== null && distanceFromBottom > scrollHeight
}
