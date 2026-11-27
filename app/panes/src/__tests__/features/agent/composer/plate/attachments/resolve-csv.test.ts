import { describe, expect, it } from 'vitest'
import {
  isCsvFile,
  resolveCsv,
  rowsToMarkdownTable,
} from '@/features/agent/composer/plate/attachments/resolve-csv'

const CSV_MAX_ROWS = 200
const CSV_MAX_COLUMNS = 20

const encode = (s: string) => new TextEncoder().encode(s)

const csvWithRowCount = (totalRows: number) =>
  Array.from({ length: totalRows }, (_, i) => `r${i},v${i}`).join('\n') + '\n'

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
    expect(resolveCsv(encode('a,b\n1,2\nx,"y|z"\n'))).toEqual({ kind: 'file' })
  })

  it('resolves a single-column CSV with no commas anywhere', () => {
    expect(resolveCsv(encode('name\nAda\nGrace\n'))).toEqual({
      kind: 'table',
      rows: [['name'], ['Ada'], ['Grace']],
    })
  })

  it('falls back to a file for empty input', () => {
    expect(resolveCsv(encode(''))).toEqual({ kind: 'file' })
  })

  it('falls back to a file on malformed CSV a real parser flags as an error', () => {
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
      const wideRow = Array.from({ length: CSV_MAX_COLUMNS + 1 }, (_, i) => `v${i}`).join(',')
      expect(resolveCsv(encode(`a,b\n${wideRow}\n`))).toEqual({ kind: 'file' })
    })
  })

  describe('ragged rows (ambient width mismatch)', () => {
    it('rejects a body row narrower than the header', () => {
      expect(resolveCsv(encode('a,b,c\n1,2\n'))).toEqual({ kind: 'file' })
    })

    it('rejects a body row wider than the header, never silently dropping cells', () => {
      const result = resolveCsv(encode('name,notes\nAda,started project, on time\nGrace,ok\n'))
      expect(result).toEqual({ kind: 'file' })
    })
  })

  describe('large-input safety', () => {
    it('rejects an oversized CSV without crashing, before any column-width computation runs', () => {
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
