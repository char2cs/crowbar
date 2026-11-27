import type { RpcFailure } from './rpc-failure'

export class PaneRequestError extends Error {
  readonly code: string

  constructor(failure: RpcFailure) {
    super(failure.message)
    this.name = 'PaneRequestError'
    this.code = failure.code
  }
}
