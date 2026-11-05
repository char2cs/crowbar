import { stripControlChars } from './control-chars'

// Some prompts embed their ANSI-coloured prompt in the OSC title; a real title never has ESC. So:
// reject ESC, strip control bytes, and reject leftover CSI bodies ("[01;32m", left when xterm drops
// the ESC) while "[WIP]" survives. '' means "no update".
export function sanitizeTerminalTitle(rawTitle: string): string {
  if (rawTitle.includes('\x1b')) return ''

  // Drop C0 controls (0–31), DEL (127), and the C1 CSI introducer (155).
  const out = stripControlChars(rawTitle).trim()

  if (/\[[\d;?]*[mhlKJH]/.test(out)) return ''

  return out
}
