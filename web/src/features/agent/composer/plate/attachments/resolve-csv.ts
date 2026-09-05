import Papa from 'papaparse'

export const CSV_MAX_ROWS = 200
export const CSV_MAX_COLUMNS = 20

export type CsvResolution = { kind: 'table'; rows: string[][] } | { kind: 'file' }

/** Two independent gates, both must pass for inline-as-table (design spec's
 *  "CSV table-vs-file" rule):
 *  1. Size — at most CSV_MAX_ROWS rows / CSV_MAX_COLUMNS columns. 200x20 is a
 *     ceiling for a table meant to be READ in a chat bubble, not a data
 *     browser; bigger belongs in a file card instead.
 *  2. Shape — parses cleanly with `papaparse` (RFC4180: quoted fields,
 *     embedded commas/quotes) with zero parse errors, and no cell contains a
 *     newline or a `|` that would corrupt GFM table syntax.
 *  Failing either gate returns `{ kind: 'file' }` — never a best-effort
 *  table, since inline CSV is delivered as ground truth the agent reads. */
export function resolveCsv(bytes: Uint8Array): CsvResolution {
  const text = new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  // `delimiter` is pinned rather than left to auto-detect: papaparse flags an
  // UndetectableDelimiter *error* for any text with no comma to guess from
  // (e.g. a valid single-column CSV), which would otherwise trip the error
  // gate below on perfectly well-formed input.
  const result = Papa.parse<string[]>(text, { skipEmptyLines: true, delimiter: ',' })
  if (result.errors.length > 0) return { kind: 'file' }

  const rows = result.data
  if (rows.length === 0) return { kind: 'file' }
  const columns = Math.max(...rows.map((row) => row.length))
  if (rows.length > CSV_MAX_ROWS || columns > CSV_MAX_COLUMNS) return { kind: 'file' }
  if (rows.some((row) => row.some((cell) => cell.includes('\n') || cell.includes('|')))) {
    return { kind: 'file' }
  }

  return { kind: 'table', rows }
}

/** Serializes resolved rows as a GFM markdown table (first row = header).
 *  Cells are NOT re-escaped for `|` — `resolveCsv` already rejects any cell
 *  containing one, so a `'table'` resolution is always safe verbatim. */
export function rowsToMarkdownTable(rows: string[][]): string {
  if (rows.length === 0) return ''
  const [header, ...body] = rows
  const columnCount = header.length
  const pad = (row: string[]) =>
    Array.from({ length: columnCount }, (_, i) => (row[i] ?? '').trim())
  const line = (row: string[]) => `| ${pad(row).join(' | ')} |`
  const separator = `| ${Array.from({ length: columnCount }, () => '---').join(' | ')} |`
  return [line(header), separator, ...body.map(line)].join('\n')
}
