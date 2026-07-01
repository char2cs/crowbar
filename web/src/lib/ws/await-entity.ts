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
//
// Snapshot-on-subscribe caveat: repo/workspace topics replay every existing row
// the instant we connect, BEFORE the freshly-created entity lands. A `match`
// that a pre-existing row satisfies (e.g. another workspace already on this
// branch, or an existing workspace under this repo) would otherwise resolve to
// the OLD entity and strand the caller in the wrong worktree. So we record the
// ids seen in the snapshot window (everything up to and including the action's
// settlement) and only resolve on a matching frame whose id is NEW.

export interface AwaitEntityOptions<T> {
  /** Hierarchical WS endpoint to subscribe, e.g. '/v0/projects'. */
  endpoint: string
  /** Returns true for the frame this caller is waiting on. */
  match: (frame: T) => boolean
  /** The mutation to fire AFTER the subscription is registered (the 202 POST). */
  action: () => Promise<void>
  /** Reject after this many ms if no matching frame arrives. Default 30s. */
  timeoutMs?: number
  /**
   * Accept a matching entity from the snapshot-on-subscribe burst — resolve on
   * the FIRST match, including pre-existing rows. Use when the awaited entity was
   * created by an EARLIER action (e.g. a side effect of a parent POST, like the
   * default workspace a repo import creates) and so is ALREADY in the snapshot
   * when we subscribe — banking it as "pre-existing" would strand the caller
   * forever. Default false: only a genuinely NEW id resolves, so a fresh create
   * on a branch that already has a workspace doesn't match the stale row.
   */
  acceptExisting?: boolean
}

const DEFAULT_TIMEOUT_MS = 30_000

export function awaitEntity<T extends { id: string }>(opts: AwaitEntityOptions<T>): Promise<T> {
  const { endpoint, match, action, timeoutMs = DEFAULT_TIMEOUT_MS, acceptExisting = false } = opts
  return new Promise<T>((resolve, reject) => {
    let settled = false
    // Ids replayed by the snapshot-on-subscribe burst. While `collecting` is
    // true a matching frame is recorded here instead of resolving — those rows
    // pre-date our mutation and must never be the answer. The action settling
    // closes the window: the freshly-created entity is broadcast only once the
    // create round-trips, i.e. at/after the POST resolves.
    const seen = new Set<string>()
    let collecting = true

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
      if (!match(frame)) return
      // Snapshot window: bank the id (it is a pre-existing row) and wait for a
      // genuinely new one. After the window closes, accept only NEW ids. Skipped
      // entirely when acceptExisting is set — the awaited entity may already be in
      // the snapshot (created by an earlier action), so the first match resolves.
      if (!acceptExisting && (collecting || seen.has(frame.id))) {
        seen.add(frame.id)
        return
      }
      finish(() => resolve(frame))
    })

    const timer = setTimeout(
      () => finish(() => reject(new Error(`timed out awaiting entity on ${endpoint}`))),
      timeoutMs,
    )

    // Fire the mutation only after the subscription is live. A rejection here
    // (e.g. a fail-fast 4xx) surfaces immediately instead of waiting for the
    // timeout. The snapshot window stays open until the action settles, so every
    // pre-existing row is banked before we start accepting new ids.
    void action()
      .then(() => {
        collecting = false
      })
      .catch((err) => finish(() => reject(err)))
  })
}
