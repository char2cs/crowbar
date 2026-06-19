import { wsManager } from './manager'
import { isReconnectSentinel } from './types'

// §4/§6 subscribe-before-POST primitive. Hierarchical mutations answer 202 with
// an empty body — the real entity (with its server-assigned id and status
// transitions) only arrives over the scoped WS broadcaster. To navigate/seed a
// caller after a mutation we therefore:
//   1. subscribe the entity stream FIRST (so no frame is dropped),
//   2. fire the mutation (the 202 POST),
//   3. resolve with the first frame that `match` accepts.
// A timeout guards against a frame that never arrives (the daemon errored after
// the 202) so callers can surface an error instead of hanging the UI.

export interface AwaitEntityOptions<T> {
  /** Hierarchical WS endpoint to subscribe, e.g. '/v0/projects'. */
  endpoint: string
  /** Returns true for the frame this caller is waiting on. */
  match: (frame: T) => boolean
  /** The mutation to fire AFTER the subscription is registered (the 202 POST). */
  action: () => Promise<void>
  /** Reject after this many ms if no matching frame arrives. Default 30s. */
  timeoutMs?: number
}

const DEFAULT_TIMEOUT_MS = 30_000

export function awaitEntity<T extends { id: string }>(
  opts: AwaitEntityOptions<T>,
): Promise<T> {
  const { endpoint, match, action, timeoutMs = DEFAULT_TIMEOUT_MS } = opts
  return new Promise<T>((resolve, reject) => {
    let settled = false

    const finish = (fn: () => void): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      unsubscribe()
      fn()
    }

    const unsubscribe = wsManager.subscribe(endpoint, (data: unknown) => {
      // The reconnect sentinel is not a DTO — ignore it.
      if (isReconnectSentinel(data)) return
      const frame = data as T
      if (!frame || typeof frame.id !== 'string') return
      if (match(frame)) finish(() => resolve(frame))
    })

    const timer = setTimeout(
      () => finish(() => reject(new Error(`timed out awaiting entity on ${endpoint}`))),
      timeoutMs,
    )

    // Fire the mutation only after the subscription is live. A rejection here
    // (e.g. a fail-fast 4xx) surfaces immediately instead of waiting for the
    // timeout.
    void action().catch((err) => finish(() => reject(err)))
  })
}
