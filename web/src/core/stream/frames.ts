import type { Problem } from '@/lib/errors/problem'

/** Mirrors `api/internal/domain/stream` by hand (index R20); its golden file is the contract. */
type EntityKind =
  'project' | 'repo' | 'workspace' | 'chat' | 'tree' | 'operation' | 'review_thread' | 'telemetry'

export type Frame =
  | { t: 'ready'; epoch: string }
  | {
      t: 'entity'
      scope: string
      kind: EntityKind
      id: string
      v: number
      data?: unknown
      gone?: true
    }
  /** `seq`: a chat scope's ledger head. */
  | { t: 'synced'; scope: string; seq?: number }
  /** A chat's ledger row; its shape is the chat feature's (`LedgerRow`). */
  | { t: 'row'; scope: string; row: Record<string, unknown> }
  /** Streamed text no row holds yet; its shape is the chat feature's (`FeedDelta`). */
  | { t: 'delta'; scope: string; delta: Record<string, unknown> }
  /** An unversioned fact about a scope (git status, a file change, diagnostics). */
  | { t: 'signal'; scope: string; kind: string; data?: unknown }
  | {
      t: 'closed'
      scope: string
      reason: 'resync' | 'deleted' | 'refused'
      /** A chat scope's resync: the last row it was sent. */
      seq?: number
      problem?: Problem
    }
  /** A refused request that names no scope to drop. */
  | { t: 'error'; problem: Problem }

/** A frame this client cannot read; `scope` names what it was about, when it said. */
export class FrameError extends Error {
  constructor(
    message: string,
    readonly scope?: string,
  ) {
    super(message)
    this.name = 'FrameError'
  }
}

const KINDS = new Set<string>([
  'project',
  'repo',
  'workspace',
  'chat',
  'tree',
  'operation',
  'review_thread',
  'telemetry',
])
const REASONS = new Set<string>(['resync', 'deleted', 'refused'])

type Check = (value: unknown) => boolean
const str: Check = (v) => typeof v === 'string'
const num: Check = (v) => typeof v === 'number' && Number.isFinite(v)
const any: Check = () => true
const yes: Check = (v) => v === true
const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v)
const problem: Check = (v) => isRecord(v) && str(v.code) && str(v.message)

/** Each type's fields; a field marked optional may be absent. */
const SHAPES: Record<Frame['t'], Record<string, { check: Check; optional?: true }>> = {
  ready: { epoch: { check: str } },
  entity: {
    scope: { check: str },
    kind: { check: (v) => str(v) && KINDS.has(v as string) },
    id: { check: str },
    v: { check: num },
    data: { check: any, optional: true },
    gone: { check: yes, optional: true },
  },
  synced: { scope: { check: str }, seq: { check: num, optional: true } },
  row: { scope: { check: str }, row: { check: isRecord } },
  delta: { scope: { check: str }, delta: { check: isRecord } },
  signal: { scope: { check: str }, kind: { check: str }, data: { check: any, optional: true } },
  closed: {
    scope: { check: str },
    reason: { check: (v) => str(v) && REASONS.has(v as string) },
    seq: { check: num, optional: true },
    problem: { check: problem, optional: true },
  },
  error: { problem: { check: problem } },
}

/** Reads one message; an unknown type or field, or a field of the wrong shape, throws. */
export function decodeFrame(raw: unknown): Frame {
  if (!isRecord(raw) || !str(raw.t) || !Object.hasOwn(SHAPES, raw.t as string))
    throw new FrameError(`stream: unknown frame ${JSON.stringify(raw)}`)
  const scope = str(raw.scope) ? (raw.scope as string) : undefined
  const shape = SHAPES[raw.t as Frame['t']]
  for (const [field, value] of Object.entries(raw)) {
    if (field === 't') continue
    const spec = shape[field]
    if (!spec?.check(value)) throw new FrameError(`stream: ${raw.t} frame: bad ${field}`, scope)
  }
  for (const [field, spec] of Object.entries(shape)) {
    if (!spec.optional && !(field in raw))
      throw new FrameError(`stream: ${raw.t} frame: no ${field}`, scope)
  }
  return raw as Frame
}
