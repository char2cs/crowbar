/** The error half of a `response` frame. */
export interface RpcFailure {
  code: string
  message: string
}
