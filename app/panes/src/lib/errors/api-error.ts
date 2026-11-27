import type { Problem, ProblemCode } from '@/lib/errors/problem'

const CATEGORY_BY_STATUS: Record<number, ProblemCode> = {
  400: 'invalid_argument',
  401: 'unauthorized',
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

function categoryFor(status: number): ProblemCode {
  return CATEGORY_BY_STATUS[status] ?? 'internal'
}

export class ApiError extends Error {
  readonly status: number
  readonly problem: Problem
  readonly data?: unknown
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

export interface WorkAtRisk {
  workspaceId: string
  branch: string
  uncommittedFiles: number
  unmergedCommits: number
}

export function workAtRiskOf(err: unknown): WorkAtRisk[] | null {
  if (!(err instanceof ApiError) || err.problem.code !== 'work_at_risk') return null
  const list = (err.data as { workAtRisk?: WorkAtRisk[] } | undefined)?.workAtRisk
  return Array.isArray(list) ? list : []
}

export function isNotFoundError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}
