/**
 * Decoding the daemon's log stream defensively: a frame this file does not
 * understand becomes a `raw` frame carrying its text, so a newer daemon shows
 * up as odd output rather than a crash.
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type FieldKind = 'text' | 'number' | 'duration' | 'bool' | 'path' | 'error'

interface LogField {
  key: string
  value: string
  kind: FieldKind
}

export interface LogRecord {
  seq: number
  /** RFC 3339 as the daemon sent it. */
  iso: string
  level: LogLevel
  component: string
  msg: string
  fields: LogField[]
}

export type LogFrame =
  | { type: 'log'; record: LogRecord }
  /** End of the replay. `reset` means the daemon restarted: what the client holds is another process's. */
  | { type: 'ready'; seq: number; reset: boolean }
  | { type: 'raw'; text: string }

const DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/
const PATH = /^(~|\/|[A-Za-z]:[\\/]|github\.com\/)/

/** How a field's value is styled: its JSON type first, then its key and shape. */
function fieldKind(key: string, value: unknown): FieldKind {
  if (typeof value === 'boolean') return 'bool'
  if (typeof value === 'number') return 'number'
  if (key === 'err' || key === 'error') return 'error'
  if (typeof value !== 'string') return 'text'
  if (DURATION.test(value)) return 'duration'
  if (PATH.test(value)) return 'path'
  return 'text'
}

function display(value: unknown): string {
  if (typeof value === 'string') return value
  if (value === null || value === undefined) return 'null'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function toLevel(raw: unknown): LogLevel {
  switch (typeof raw === 'string' ? raw.toLowerCase() : '') {
    case 'debug':
    case 'trace':
      return 'debug'
    case 'warn':
    case 'warning':
      return 'warn'
    case 'error':
    case 'fatal':
    case 'panic':
      return 'error'
    default:
      return 'info'
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function parseLog(frame: Record<string, unknown>): LogFrame | null {
  if (typeof frame.seq !== 'number' || typeof frame.msg !== 'string') return null
  const fields = isObject(frame.fields) ? frame.fields : {}
  return {
    type: 'log',
    record: {
      seq: frame.seq,
      iso: typeof frame.time === 'string' ? frame.time : '',
      level: toLevel(frame.level),
      component: typeof frame.component === 'string' ? frame.component : '',
      msg: frame.msg,
      fields: Object.entries(fields).map(([key, value]) => ({
        key,
        value: display(value),
        kind: fieldKind(key, value),
      })),
    },
  }
}

/** Decodes one frame: the text the socket delivered, or an object already parsed. */
export function parseLogFrame(data: unknown): LogFrame {
  let value: unknown = data
  if (typeof data === 'string') {
    try {
      value = JSON.parse(data)
    } catch {
      return { type: 'raw', text: data }
    }
  }
  if (!isObject(value))
    return { type: 'raw', text: typeof data === 'string' ? data : display(data) }

  const text = typeof data === 'string' ? data : JSON.stringify(value)
  switch (value.type) {
    case 'log':
      return parseLog(value) ?? { type: 'raw', text }
    case 'ready':
      return typeof value.seq === 'number'
        ? { type: 'ready', seq: value.seq, reset: value.reset === true }
        : { type: 'raw', text }
    default:
      return { type: 'raw', text }
  }
}

function pad(n: number, width = 2): string {
  return String(n).padStart(width, '0')
}

/** `HH:mm:ss.SSS` in the viewer's zone; the record's own text when it is not a date. */
export function formatLogTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}

export interface JsonLine {
  key: string
  /** Strings are quoted, so the expanded view reads as JSON. */
  value: string
  kind: FieldKind
}

/** The record as the daemon would print it, one entry per key: what a click on a line reveals. */
export function jsonLines(record: LogRecord): JsonLine[] {
  const lines: JsonLine[] = [
    { key: 'time', value: JSON.stringify(record.iso), kind: 'text' },
    { key: 'level', value: JSON.stringify(record.level), kind: 'text' },
  ]
  if (record.component)
    lines.push({ key: 'component', value: JSON.stringify(record.component), kind: 'text' })
  lines.push({ key: 'msg', value: JSON.stringify(record.msg), kind: 'text' })
  for (const field of record.fields) {
    const quoted =
      field.kind === 'number' || field.kind === 'bool' ? field.value : JSON.stringify(field.value)
    lines.push({ key: field.key, value: quoted, kind: field.kind })
  }
  return lines
}
