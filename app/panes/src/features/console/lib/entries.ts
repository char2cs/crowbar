import type { LogRecord } from './frames'

export type ConsoleEntry =
  | { id: number; kind: 'log'; record: LogRecord }
  | { id: number; kind: 'raw'; text: string }
  /** The daemon restarted: the lines above this one belong to another process. */
  | { id: number; kind: 'restarted' }

/** An entry before it has been given an id. */
export type NewEntry =
  { kind: 'log'; record: LogRecord } | { kind: 'raw'; text: string } | { kind: 'restarted' }
