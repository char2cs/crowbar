import { MAX_TITLE_BYTES } from './sdk-limits'

const ELLIPSIS = '...'
const encoder = new TextEncoder()

const byteLength = (text: string) => encoder.encode(text).length

/** Cleans a title for the shell tab: control characters become spaces, and a title over the shell's byte limit is cut with an ellipsis. */
export function fitTitle(title: string): string {
  const clean = title.replace(/\p{Cc}/gu, ' ').trim()
  if (byteLength(clean) <= MAX_TITLE_BYTES) return clean
  let cut = clean
  while (byteLength(cut) > MAX_TITLE_BYTES - ELLIPSIS.length) cut = cut.slice(0, -1)
  return `${cut}${ELLIPSIS}`
}
