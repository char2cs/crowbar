import { describe, it, expect } from 'vitest'
import {
  selectionTextPreservingWraps,
  type SelectionLine,
  type SelectionSource,
} from '@/features/terminal/utils/selection-text'

const NBSP = '\u00A0'

// A stand-in for xterm's IBuffer. Rows are given EXACTLY as the daemon paints
// them — one string per VISUAL row — because that is the shape the real bug
// lives in: the daemon redraws every row at column 1, so a row that is only a
// continuation of the one above it arrives looking like a line of its own.
//
// `isWrapped` defaults to false on every row for the same reason: the client's
// xterm never auto-wrapped, so it never set the flag. Pass wrapped: [...] to
// model the one place it IS set (scrollback replayed as whole logical lines).
function makeSource(rows: string[], cols: number, wrapped: boolean[] = []): SelectionSource {
  const lines: SelectionLine[] = rows.map((row, y) => {
    const cells = row.padEnd(cols, ' ').slice(0, cols).split('')
    return {
      isWrapped: wrapped[y] ?? false,
      translateToString: (trimRight?: boolean, start = 0, end = cols) => {
        const text = cells.slice(start, end).join('')
        return trimRight ? text.replace(/ +$/, '') : text
      },
      // An untouched cell reads back as '' in xterm — that is exactly how a row
      // that ended early is told apart from one filled to the last column.
      getCell: (x: number) => ({ getChars: () => (cells[x] === ' ' ? '' : (cells[x] ?? '')) }),
    }
  })
  return { cols, getLine: (y: number) => lines[y] }
}

const all = (rows: string[], cols: number) => ({
  start: { x: 0, y: 0 },
  end: { x: cols, y: rows.length - 1 },
})

describe('selectionTextPreservingWraps', () => {
  // THE BUG. A single 300-char line printed by the shell occupies two visual
  // rows. xterm's own selectionText puts a newline between them because the
  // daemon's row-addressed redraw left isWrapped false, so a copied path/URL
  // comes back snapped in half at the terminal width.
  it('joins a row that is filled to the last column with the row below it', () => {
    const cols = 10
    const rows = ['XXXXXXXXXX', 'XXXXX']
    expect(selectionTextPreservingWraps(makeSource(rows, cols), all(rows, cols))).toBe(
      'XXXXXXXXXXXXXXX',
    )
  })

  it('keeps the newline when the row above ended before the last column', () => {
    const cols = 10
    const rows = ['echo hi', 'hi']
    expect(selectionTextPreservingWraps(makeSource(rows, cols), all(rows, cols))).toBe(
      'echo hi\nhi',
    )
  })

  it('joins a run of three rows from one long logical line', () => {
    const cols = 5
    const rows = ['aaaaa', 'bbbbb', 'ccc']
    expect(selectionTextPreservingWraps(makeSource(rows, cols), all(rows, cols))).toBe(
      'aaaaabbbbbccc',
    )
  })

  it('still honours isWrapped when xterm did record it (replayed scrollback)', () => {
    const cols = 10
    // Row 0 ends early yet row 1 is flagged as its continuation — trust the flag.
    const rows = ['short', 'tail']
    const source = makeSource(rows, cols, [false, true])
    expect(selectionTextPreservingWraps(source, all(rows, cols))).toBe('shorttail')
  })

  it('trims the trailing blanks off a row that ended early', () => {
    const cols = 10
    const rows = ['DONE', 'NEXT']
    expect(selectionTextPreservingWraps(makeSource(rows, cols), all(rows, cols))).toBe('DONE\nNEXT')
  })

  it('slices the first and last rows to the selected columns', () => {
    const cols = 10
    const rows = ['0123456789', 'abcdefghij', 'zzzzzzzzzz']
    const source = makeSource(rows, cols)
    // Start mid-row-0, end mid-row-2. Rows 0 and 1 are both full, so the whole
    // thing is one logical line and no newline belongs anywhere in it.
    expect(
      selectionTextPreservingWraps(source, { start: { x: 4, y: 0 }, end: { x: 3, y: 2 } }),
    ).toBe('456789abcdefghijzzz')
  })

  it('reads a selection contained in one row', () => {
    const cols = 10
    const source = makeSource(['0123456789'], cols)
    expect(
      selectionTextPreservingWraps(source, { start: { x: 2, y: 0 }, end: { x: 5, y: 0 } }),
    ).toBe('234')
  })

  // A drag that runs bottom-to-top hands back start after end; xterm's own
  // final-selection getters already order them, but ordering here too keeps the
  // function correct against any caller.
  it('orders a reversed range before reading it', () => {
    const cols = 10
    const rows = ['first', 'second']
    const source = makeSource(rows, cols)
    expect(
      selectionTextPreservingWraps(source, { start: { x: 6, y: 1 }, end: { x: 0, y: 0 } }),
    ).toBe('first\nsecond')
  })

  it('returns empty for a zero-width selection', () => {
    const source = makeSource(['abc'], 10)
    expect(
      selectionTextPreservingWraps(source, { start: { x: 2, y: 0 }, end: { x: 2, y: 0 } }),
    ).toBe('')
  })

  it('returns empty when the range points past the buffer', () => {
    const source = makeSource(['abc'], 10)
    expect(
      selectionTextPreservingWraps(source, { start: { x: 0, y: 9 }, end: { x: 3, y: 9 } }),
    ).toBe('')
  })

  // xterm normalizes NBSP on the way to the clipboard; a terminal that renders
  // padding as NBSP would otherwise paste invisible non-breaking spaces.
  it('rewrites non-breaking spaces to plain spaces', () => {
    const source = makeSource([`a${NBSP}b`], 10)
    expect(
      selectionTextPreservingWraps(source, { start: { x: 0, y: 0 }, end: { x: 3, y: 0 } }),
    ).toBe('a b')
  })
})
