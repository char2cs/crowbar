import type { Problem } from '@/lib/errors/problem'

/** An operation that settled as failed; the operations watcher already showed it. */
export class OperationFailedError extends Error {
  readonly problem: Problem
  readonly operationId: string
  constructor(problem: Problem, operationId: string) {
    super(problem.message)
    this.name = 'OperationFailedError'
    this.problem = problem
    this.operationId = operationId
  }
}
