import { describe, it, expect } from 'vitest'
import {
  analyzeTerminalPaste,
  LARGE_PASTE_CHAR_THRESHOLD,
} from '@/features/terminal/utils/paste-guard'

describe('analyzeTerminalPaste', () => {
  it('does not require confirmation for short single-line text', () => {
    const result = analyzeTerminalPaste('ls -la')
    expect(result.requiresConfirmation).toBe(false)
    expect(result.lineCount).toBe(1)
    expect(result.normalizedText).toBe('ls -la')
  })

  // BUG-016: any newline in the paste can execute commands immediately, so
  // even a two-line paste must confirm.
  it('requires confirmation when the text contains a newline', () => {
    const result = analyzeTerminalPaste('echo a\necho b')
    expect(result.requiresConfirmation).toBe(true)
    expect(result.lineCount).toBe(2)
  })

  it('requires confirmation for a single trailing newline (executes on paste)', () => {
    const result = analyzeTerminalPaste('rm -rf /tmp/x\n')
    expect(result.requiresConfirmation).toBe(true)
    expect(result.lineCount).toBe(2)
  })

  it('normalises CRLF to LF and counts lines on the normalised text', () => {
    const result = analyzeTerminalPaste('a\r\nb\r\nc')
    expect(result.normalizedText).toBe('a\nb\nc')
    expect(result.lineCount).toBe(3)
    expect(result.requiresConfirmation).toBe(true)
  })

  it('requires confirmation for very large single-line pastes', () => {
    const result = analyzeTerminalPaste('x'.repeat(LARGE_PASTE_CHAR_THRESHOLD))
    expect(result.lineCount).toBe(1)
    expect(result.requiresConfirmation).toBe(true)
  })

  it('keeps just-below-threshold single-line pastes unconfirmed', () => {
    const result = analyzeTerminalPaste('x'.repeat(LARGE_PASTE_CHAR_THRESHOLD - 1))
    expect(result.requiresConfirmation).toBe(false)
  })
})
