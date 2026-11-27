import Papa from 'papaparse'

const CSV_MAX_ROWS = 200
const CSV_MAX_COLUMNS = 20

export type CsvResolution = { kind: 'table'; rows: string[][] } | { kind: 'file' }

export function isCsvFile(file: File): boolean {
  return file.name.toLowerCase().endsWith('.csv') || file.type === 'text/csv'
}

export function resolveCsv(bytes: Uint8Array): CsvResolution {
  const text = new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  const result = Papa.parse<string[]>(text, { skipEmptyLines: true, delimiter: ',' })
  if (result.errors.length > 0) return { kind: 'file' }

  const rows = result.data
  if (rows.length === 0) return { kind: 'file' }
  if (rows.length > CSV_MAX_ROWS) return { kind: 'file' }

  const columnCount = rows[0].length
  if (columnCount > CSV_MAX_COLUMNS) return { kind: 'file' }
  if (rows.some((row) => row.length !== columnCount)) return { kind: 'file' }
  if (rows.some((row) => row.some((cell) => cell.includes('\n') || cell.includes('|')))) {
    return { kind: 'file' }
  }

  return { kind: 'table', rows }
}

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
