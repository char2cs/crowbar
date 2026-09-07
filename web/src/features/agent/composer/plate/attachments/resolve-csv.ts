import Papa from 'papaparse'

export const CSV_MAX_ROWS = 200
export const CSV_MAX_COLUMNS = 20

export type CsvResolution = { kind: 'table'; rows: string[][] } | { kind: 'file' }

/** A `.csv` file by either signal a browser gives one: the extension (always
 *  present) or `contentType` — which in practice is `text/csv` in some
 *  browsers/OSes and an empty string in others (no OS association for the
 *  extension), never something to rely on alone. */
export function isCsvFile(file: File): boolean {
  return file.name.toLowerCase().endsWith('.csv') || file.type === 'text/csv'
}

/** Two independent gates, both must pass for inline-as-table (design spec's
 *  "CSV table-vs-file" rule):
 *  1. Size — at most CSV_MAX_ROWS rows / CSV_MAX_COLUMNS columns. 200x20 is a
 *     ceiling for a table meant to be READ in a chat bubble, not a data
 *     browser; bigger belongs in a file card instead.
 *  2. Shape — parses cleanly with `papaparse` (RFC4180: quoted fields,
 *     embedded commas/quotes) with zero parse errors, every row has exactly
 *     the header's column count (a ragged CSV is rejected outright, never
 *     padded or truncated), and no cell contains a newline or a `|` that
 *     would corrupt GFM table syntax.
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
  // Row-count gate first, before any per-row inspection: a column count
  // computed via `Math.max(...rows.map((r) => r.length))` spreads one array
  // element per row into a function call, which overflows the call stack on
  // an oversized CSV (confirmed crashing well under a million rows) —
  // exactly the input this gate exists to reject. `rows[0].length` below is
  // O(1) and the uniform-width scan after it is a plain loop, so neither can
  // reintroduce that crash regardless of size.
  if (rows.length > CSV_MAX_ROWS) return { kind: 'file' }

  const columnCount = rows[0].length
  if (columnCount > CSV_MAX_COLUMNS) return { kind: 'file' }
  // Every row must match the header's width exactly. A row NARROWER or
  // WIDER than the header is rejected rather than padded or truncated —
  // `rowsToMarkdownTable` sizes its output from the header alone, so a wider
  // row would otherwise have its extra cells silently dropped with no error
  // and no visual signal, in data the agent reads as ground truth.
  if (rows.some((row) => row.length !== columnCount)) return { kind: 'file' }
  if (rows.some((row) => row.some((cell) => cell.includes('\n') || cell.includes('|')))) {
    return { kind: 'file' }
  }

  return { kind: 'table', rows }
}

/** Serializes resolved rows as a GFM markdown table (first row = header).
 *  Assumes resolveCsv's invariants — no `|`/newline in any cell, every row
 *  exactly the header's width — so a `'table'` resolution is always safe
 *  verbatim; called directly with a row wider than the header, outside that
 *  contract, this drops the row's extra cells silently. */
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
