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

export function fitTitle(title: string): string {
  return fitBytes(title.replace(/\p{Cc}/gu, ' ').trim(), MAX_TITLE_BYTES)
}

export function fitDisplayText(text: string): string {
  return fitBytes(text, MAX_DISPLAY_TEXT_BYTES)
}
