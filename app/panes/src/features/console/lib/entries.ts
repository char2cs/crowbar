import type { LogRecord } from './frames'

export type ConsoleEntry =
  | { id: number; kind: 'log'; record: LogRecord }
  | { id: number; kind: 'raw'; text: string }
  | { id: number; kind: 'restarted' }

export type NewEntry =
  { kind: 'log'; record: LogRecord } | { kind: 'raw'; text: string } | { kind: 'restarted' }
