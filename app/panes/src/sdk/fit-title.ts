import { MAX_DISPLAY_TEXT_BYTES, MAX_TITLE_BYTES } from './sdk-limits'

const ELLIPSIS = '...'
const encoder = new TextEncoder()

const byteLength = (text: string) => encoder.encode(text).length

function fitBytes(text: string, limit: number): string {
  if (byteLength(text) <= limit) return text
  let cut = text
  while (byteLength(cut) > limit - ELLIPSIS.length) cut = cut.slice(0, -1)
  return `${cut}${ELLIPSIS}`
}

/** Cleans a title for the shell tab: control characters become spaces, and a title over the shell's byte limit is cut with an ellipsis. */
export function fitTitle(title: string): string {
  return fitBytes(title.replace(/\p{Cc}/gu, ' ').trim(), MAX_TITLE_BYTES)
}

/** Cuts toast text over the shell's byte limit, with an ellipsis. */
export function fitDisplayText(text: string): string {
  return fitBytes(text, MAX_DISPLAY_TEXT_BYTES)
}
