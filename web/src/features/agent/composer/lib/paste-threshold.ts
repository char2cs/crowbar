/**
 * Paste-to-pill threshold — the design spec's own proposed default, adopted
 * as-is: long enough that a normal sentence or short snippet never collapses
 * (nobody wants "here's the fix:" turning into a pill), short enough that a
 * genuinely large paste — a stack trace, a file dump — collapses instead of
 * flooding the box as unreadable wrapped prose. Tunable, not architectural;
 * see the design spec's "Thresholds & fallback rules".
 */
export const PASTE_CHAR_THRESHOLD = 400
export const PASTE_LINE_THRESHOLD = 6

/** Either threshold alone is enough — a long single line (a minified blob,
 *  a URL-encoded token) is exactly as unreadable inline as six short ones. */
export function shouldWrapAsTextAttachment(text: string): boolean {
  if (text.length > PASTE_CHAR_THRESHOLD) return true
  const lineCount = text.split('\n').length
  return lineCount > PASTE_LINE_THRESHOLD
}
