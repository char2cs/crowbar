import type { RpcFailure } from './rpc-failure'

/** A call the shell or the pane's backend answered with an error; `code` is the machine-readable cause. */
export class PaneRequestError extends Error {
  readonly code: string

  constructor(failure: RpcFailure) {
    super(failure.message)
    this.name = 'PaneRequestError'
    this.code = failure.code
  }
}
