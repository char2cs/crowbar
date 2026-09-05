import { describe, expect, it } from 'vitest'
import {
  CSV_MAX_COLUMNS,
  CSV_MAX_ROWS,
  isCsvFile,
  resolveCsv,
  rowsToMarkdownTable,
} from '@/features/agent/composer/plate/attachments/resolve-csv'

const encode = (s: string) => new TextEncoder().encode(s)

/** Builds a CSV with exactly `totalRows` rows (no distinct header — resolveCsv's
 *  size gate counts rows uniformly), two columns each. */
const csvWithRowCount = (totalRows: number) =>
  Array.from({ length: totalRows }, (_, i) => `r${i},v${i}`).join('\n') + '\n'

/** Builds a single-row CSV with exactly `totalColumns` columns. */
const csvWithColumnCount = (totalColumns: number) =>
  Array.from({ length: totalColumns }, (_, i) => `c${i}`).join(',') + '\n'

describe('resolveCsv', () => {
  it('parses a small, well-formed CSV into a table', () => {
    expect(resolveCsv(encode('name,age\nAda,36\nGrace,85\n'))).toEqual({
      kind: 'table',
      rows: [
        ['name', 'age'],
        ['Ada', '36'],
        ['Grace', '85'],
      ],
    })
  })

  it('handles RFC4180 quoted fields with embedded commas and quotes', () => {
    expect(resolveCsv(encode('name,note\n"Smith, John","said ""hi"""\n'))).toEqual({
      kind: 'table',
      rows: [
        ['name', 'note'],
        ['Smith, John', 'said "hi"'],
      ],
    })
  })

  it('falls back to a file when a cell contains a `|`, which would corrupt GFM table syntax', () => {
    expect(resolveCsv(encode('name,formula\nx,"a|b"\n'))).toEqual({ kind: 'file' })
  })

  it('falls back to a file when a quoted cell embeds a real newline', () => {
    expect(resolveCsv(encode('name,note\nx,"line one\nline two"\n'))).toEqual({ kind: 'file' })
  })

  it('catches a `|`/newline violation that only appears in a later row, not just the first', () => {
    // Header row is clean; the violation is in row index 2 (third row).
    expect(resolveCsv(encode('a,b\n1,2\nx,"y|z"\n'))).toEqual({ kind: 'file' })
  })

  it('resolves a single-column CSV with no commas anywhere', () => {
    // Regression: papaparse auto-detects the delimiter when none is given,
    // and flags an UndetectableDelimiter *error* whenever the text has no
    // comma to detect from — even though it still defaults to ',' and
    // parses correctly. Without pinning `delimiter: ','` explicitly, this
    // would wrongly reject every valid single-column CSV as `{kind:'file'}`.
    expect(resolveCsv(encode('name\nAda\nGrace\n'))).toEqual({
      kind: 'table',
      rows: [['name'], ['Ada'], ['Grace']],
    })
  })

  it('falls back to a file for empty input', () => {
    expect(resolveCsv(encode(''))).toEqual({ kind: 'file' })
  })

  it('falls back to a file on malformed CSV a real parser flags as an error', () => {
    // papaparse 5.7.0 flags this unterminated quoted field as a real
    // `MissingQuotes` parse error (verified against the installed version).
    expect(resolveCsv(encode('name,note\n"unterminated,x\n'))).toEqual({ kind: 'file' })
  })

  describe('row cutoff', () => {
    it('accepts exactly CSV_MAX_ROWS rows', () => {
      const result = resolveCsv(encode(csvWithRowCount(CSV_MAX_ROWS)))
      expect(result.kind).toBe('table')
      expect(result.kind === 'table' && result.rows).toHaveLength(CSV_MAX_ROWS)
    })

    it('accepts one row under CSV_MAX_ROWS', () => {
      const result = resolveCsv(encode(csvWithRowCount(CSV_MAX_ROWS - 1)))
      expect(result.kind).toBe('table')
      expect(result.kind === 'table' && result.rows).toHaveLength(CSV_MAX_ROWS - 1)
    })

    it('falls back to a file one row over CSV_MAX_ROWS', () => {
      expect(resolveCsv(encode(csvWithRowCount(CSV_MAX_ROWS + 1)))).toEqual({ kind: 'file' })
    })
  })

  describe('column cutoff', () => {
    it('accepts exactly CSV_MAX_COLUMNS columns', () => {
      const result = resolveCsv(encode(csvWithColumnCount(CSV_MAX_COLUMNS)))
      expect(result.kind).toBe('table')
      expect(result.kind === 'table' && result.rows[0]).toHaveLength(CSV_MAX_COLUMNS)
    })

    it('accepts one column under CSV_MAX_COLUMNS', () => {
      const result = resolveCsv(encode(csvWithColumnCount(CSV_MAX_COLUMNS - 1)))
      expect(result.kind).toBe('table')
      expect(result.kind === 'table' && result.rows[0]).toHaveLength(CSV_MAX_COLUMNS - 1)
    })

    it('falls back to a file one column over CSV_MAX_COLUMNS', () => {
      const header = Array.from({ length: CSV_MAX_COLUMNS + 1 }, (_, i) => `c${i}`).join(',')
      expect(resolveCsv(encode(`${header}\n`))).toEqual({ kind: 'file' })
    })

    it('rejects a ragged row over the column cutoff even when the header is short', () => {
      // Header has 2 columns; a later row has CSV_MAX_COLUMNS + 1 — caught by
      // the ragged-row check below, not by measuring the header's own width.
      const wideRow = Array.from({ length: CSV_MAX_COLUMNS + 1 }, (_, i) => `v${i}`).join(',')
      expect(resolveCsv(encode(`a,b\n${wideRow}\n`))).toEqual({ kind: 'file' })
    })
  })

  describe('ragged rows (ambient width mismatch)', () => {
    it('rejects a body row narrower than the header', () => {
      expect(resolveCsv(encode('a,b,c\n1,2\n'))).toEqual({ kind: 'file' })
    })

    it('rejects a body row wider than the header, never silently dropping cells', () => {
      // Regression: an unquoted field that itself contains a literal comma
      // (a common real-world CSV malformation) produces a row wider than the
      // header. resolveCsv must reject this outright rather than resolving
      // to a table whose rendering would truncate the row's trailing cells.
      const result = resolveCsv(encode('name,notes\nAda,started project, on time\nGrace,ok\n'))
      expect(result).toEqual({ kind: 'file' })
    })
  })

  describe('large-input safety', () => {
    it('rejects an oversized CSV without crashing, before any column-width computation runs', () => {
      // 150,000 rows is well past the row-count gate's cutoff (200) and past
      // the size at which spreading one array element per row into
      // `Math.max(...)` overflows the call stack under V8 (confirmed
      // crashing under 131,000) — the exact bug this test guards against.
      // Parsing this many rows is still fast (tens of milliseconds), so this
      // stays a fast unit test while proving the row-count gate returns
      // `{kind:'file'}` well before any per-row column-width logic runs.
      const rowCount = 150_000
      const text = Array.from({ length: rowCount }, (_, i) => `r${i},v${i}`).join('\n') + '\n'
      expect(() => resolveCsv(encode(text))).not.toThrow()
      expect(resolveCsv(encode(text))).toEqual({ kind: 'file' })
    })

    it('rejects a single row far over the column cutoff without crashing', () => {
      const columnCount = 150_000
      const header = Array.from({ length: columnCount }, (_, i) => `c${i}`).join(',')
      expect(() => resolveCsv(encode(`${header}\n`))).not.toThrow()
      expect(resolveCsv(encode(`${header}\n`))).toEqual({ kind: 'file' })
    })
  })
})

describe('rowsToMarkdownTable', () => {
  it('serializes rows as a GFM table with a header separator', () => {
    expect(
      rowsToMarkdownTable([
        ['name', 'age'],
        ['Ada', '36'],
      ]),
    ).toBe('| name | age |\n| --- | --- |\n| Ada | 36 |')
  })

  it('pads a ragged body row out to the header column count', () => {
    expect(rowsToMarkdownTable([['name', 'age'], ['Ada']])).toBe(
      '| name | age |\n| --- | --- |\n| Ada |  |',
    )
  })

  it('trims whitespace around cell values', () => {
    expect(
      rowsToMarkdownTable([
        [' name ', ' age '],
        [' Ada ', ' 36 '],
      ]),
    ).toBe('| name | age |\n| --- | --- |\n| Ada | 36 |')
  })

  it('returns an empty string for no rows', () => {
    expect(rowsToMarkdownTable([])).toBe('')
  })
})

describe('isCsvFile', () => {
  it('recognizes a .csv extension with a proper text/csv contentType', () => {
    expect(isCsvFile(new File(['a,b'], 'report.csv', { type: 'text/csv' }))).toBe(true)
  })

  // Some browsers/OSes have no association for `.csv` and report an empty
  // `type` — the extension alone must still be enough.
  it('recognizes a .csv extension with an empty contentType', () => {
    expect(isCsvFile(new File(['a,b'], 'report.csv', { type: '' }))).toBe(true)
  })

  it('is case-insensitive on the extension', () => {
    expect(isCsvFile(new File(['a,b'], 'REPORT.CSV', { type: '' }))).toBe(true)
  })

  it('recognizes a text/csv contentType even without a .csv extension', () => {
    expect(isCsvFile(new File(['a,b'], 'report', { type: 'text/csv' }))).toBe(true)
  })

  it('is false for an unrelated file', () => {
    expect(isCsvFile(new File(['hi'], 'notes.txt', { type: 'text/plain' }))).toBe(false)
  })
})
