import Papa from 'papaparse'

export const CSV_MAX_ROWS = 200
export const CSV_MAX_COLUMNS = 20

export type CsvResolution = { kind: 'table'; rows: string[][] } | { kind: 'file' }

/** Extension or `text/csv`: some OSes report an empty type for `.csv`. */
export function isCsvFile(file: File): boolean {
  return file.name.toLowerCase().endsWith('.csv') || file.type === 'text/csv'
}

/** Inline only when small and cleanly shaped; otherwise a file, never a
 *  best-effort table — the agent reads inline CSV as ground truth. */
export function resolveCsv(bytes: Uint8Array): CsvResolution {
  const text = new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  // Pinned: auto-detect errors on a valid single-column CSV.
  const result = Papa.parse<string[]>(text, { skipEmptyLines: true, delimiter: ',' })
  if (result.errors.length > 0) return { kind: 'file' }

  const rows = result.data
  if (rows.length === 0) return { kind: 'file' }
  // Row count first, and no spread over rows: spreading a huge CSV into a call overflows the stack.
  if (rows.length > CSV_MAX_ROWS) return { kind: 'file' }

  const columnCount = rows[0].length
  if (columnCount > CSV_MAX_COLUMNS) return { kind: 'file' }
  // Ragged rows are rejected: the table is sized from the header and would
  // silently drop extra cells.
  if (rows.some((row) => row.length !== columnCount)) return { kind: 'file' }
  if (rows.some((row) => row.some((cell) => cell.includes('\n') || cell.includes('|')))) {
    return { kind: 'file' }
  }

  return { kind: 'table', rows }
}

/** Assumes resolveCsv's invariants: no `|`/newline in a cell, every row the header's width. */
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
