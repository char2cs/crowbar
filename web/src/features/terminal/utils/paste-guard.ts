// Decides whether a clipboard paste into the terminal needs an explicit
// user confirmation before it reaches the PTY. Any text containing a newline
// can execute commands immediately on paste (BUG-016), so multi-line pastes
// always confirm; very large single-line pastes confirm too.

export const LARGE_PASTE_CHAR_THRESHOLD = 1000

export interface PasteAnalysis {
  normalizedText: string
  lineCount: number
  requiresConfirmation: boolean
}

export function analyzeTerminalPaste(text: string): PasteAnalysis {
  const normalizedText = text.replace(/\r\n/g, '\n')
  const lineCount = normalizedText.split('\n').length
  return {
    normalizedText,
    lineCount,
    requiresConfirmation: lineCount > 1 || normalizedText.length >= LARGE_PASTE_CHAR_THRESHOLD,
  }
}
