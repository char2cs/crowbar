const PASTE_CHAR_THRESHOLD = 400
const PASTE_LINE_THRESHOLD = 6

export function shouldWrapAsTextAttachment(text: string): boolean {
  if (text.length > PASTE_CHAR_THRESHOLD) return true
  const lineCount = text.split('\n').length
  return lineCount > PASTE_LINE_THRESHOLD
}
