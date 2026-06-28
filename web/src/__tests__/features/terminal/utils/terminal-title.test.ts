import { describe, it, expect } from 'vitest'
import { sanitizeTerminalTitle } from '@/features/terminal/utils/terminal-title'

describe('sanitizeTerminalTitle', () => {
  it('passes through a clean app-set title', () => {
    expect(sanitizeTerminalTitle('vim README.md')).toBe('vim README.md')
    expect(sanitizeTerminalTitle('✻ Claude Code')).toBe('✻ Claude Code')
    expect(sanitizeTerminalTitle('ls -G -larph')).toBe('ls -G -larph')
  })

  it('rejects the exact garbled prompt-in-OSC title seen on restore (regression)', () => {
    // The shell encoded its full ANSI-colored prompt inside the OSC title; on a
    // restore replay the last such sequence won and corrupted the tab name.
    const raw =
      'crowbar [\x1b[01;32m0\x1b[00m] % \x1b[K\x1b[?1h\x1b=\x1b[?2004h ls -larph' +
      '\x1b[?1l\x1b>\x1b[?2004l \x1b]2;ls -G -larph'
    expect(sanitizeTerminalTitle(raw)).toBe('')
  })

  it('rejects any title containing a raw ESC (two-byte Fe sequences too)', () => {
    expect(sanitizeTerminalTitle('foo\x1b=bar')).toBe('') // DECKPAM
    expect(sanitizeTerminalTitle('foo\x1b>bar')).toBe('') // DECKPNM
    expect(sanitizeTerminalTitle('\x1b[31mred\x1b[0m')).toBe('')
  })

  it('strips stray C0/DEL/C1 control bytes when there is no ESC', () => {
    expect(sanitizeTerminalTitle('a\x07b\x7fc')).toBe('abc') // BEL + DEL
    expect(sanitizeTerminalTitle('tail\x00')).toBe('tail')
  })

  it('returns empty for blank / control-only input', () => {
    expect(sanitizeTerminalTitle('')).toBe('')
    expect(sanitizeTerminalTitle('   ')).toBe('')
    expect(sanitizeTerminalTitle('\x07\x00')).toBe('')
  })
})
