// Long enough that a sentence never collapses into a pill, short enough that a
// stack trace or file dump does. Tunable.
export const PASTE_CHAR_THRESHOLD = 400
export const PASTE_LINE_THRESHOLD = 6

/** Either alone is enough: a minified single line is as unreadable as six short ones. */
export function shouldWrapAsTextAttachment(text: string): boolean {
  if (text.length > PASTE_CHAR_THRESHOLD) return true
  const lineCount = text.split('\n').length
  return lineCount > PASTE_LINE_THRESHOLD
}
