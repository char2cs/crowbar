import { stripControlChars } from './control-chars'

export function sanitizeTerminalTitle(rawTitle: string): string {
  if (rawTitle.includes('\x1b')) return ''

  const out = stripControlChars(rawTitle).trim()

  if (/\[[\d;?]*[mhlKJH]/.test(out)) return ''

  return out
}
