import type { Problem, ProblemCode } from '@/lib/errors/problem'

const CATEGORY_BY_STATUS: Record<number, ProblemCode> = {
  400: 'invalid_argument',
  403: 'forbidden',
  404: 'not_found',
  409: 'conflict',
  413: 'too_large',
  422: 'unprocessable',
  424: 'failed_dependency',
  502: 'bad_gateway',
  503: 'unavailable',
  504: 'timeout',
}

/** The category code of a bare status, for a refusal that names no code. */
function categoryFor(status: number): ProblemCode {
  return CATEGORY_BY_STATUS[status] ?? 'internal'
}

/** A refusal from the daemon (or the desktop proxy in front of it): the HTTP
 *  status, and the Problem every surface renders it from. */
export class ApiError extends Error {
  readonly status: number
  readonly problem: Problem
  /** The error envelope's `data`, for the few refusals that carry detail. */
  readonly data?: unknown
  /** The host the failed request or stream targeted, 'local' for the local daemon. */
  readonly hostId?: string
  constructor(message: string, status: number, code?: string, data?: unknown, hostId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.problem = { code: (code as ProblemCode | undefined) ?? categoryFor(status), message }
    this.data = data
    this.hostId = hostId
  }
}

/** What deleting one workspace would destroy that exists nowhere else. */
export interface WorkAtRisk {
  workspaceId: string
  branch: string
  uncommittedFiles: number
  unmergedCommits: number
}

/** The work a delete was refused over, or null for any other failure. The daemon refuses such a
 *  delete until resent with the discard header, which only a client that showed this may send. */
export function workAtRiskOf(err: unknown): WorkAtRisk[] | null {
  if (!(err instanceof ApiError) || err.problem.code !== 'work_at_risk') return null
  const list = (err.data as { workAtRisk?: WorkAtRisk[] } | undefined)?.workAtRisk
  return Array.isArray(list) ? list : []
}

export function isNotFoundError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}
