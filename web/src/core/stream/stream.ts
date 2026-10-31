import { report } from '@/lib/errors/report'
import { wsManager } from '@/lib/ws/manager'
import { decodeFrame, FrameError, type Frame } from '@/core/stream/frames'

/** What the store owning a scope kind hears; core imports no feature, so stores register. */
export interface ScopeKindHandler {
  /** Where a held scope resumes from (chat scopes). */
  since?(key: string): number
  /** A sub for `scope` was sent: its snapshot follows. */
  subscribed?(scope: string): void
  /** Every frame of a scope of this kind. */
  frame(f: Frame): void
  /** A new daemon epoch: every version held is void. */
  epoch?(epoch: string): void
}

const ENDPOINT = '/v0/stream'

const handlers = new Map<string, ScopeKindHandler>()
const holds = new Map<string, number>()
/** The current socket said ready, so a sub sent now is answered. */
let ready = false
let epoch: string | undefined

function kindOf(scope: string): string {
  const colon = scope.indexOf(':')
  return colon < 0 ? scope : scope.slice(0, colon)
}

function keyOf(scope: string): string {
  const colon = scope.indexOf(':')
  return colon < 0 ? '' : scope.slice(colon + 1)
}

/** A store claims the frames of `kind`; returns the unregister. */
export function registerScopeKind(kind: string, h: ScopeKindHandler): () => void {
  handlers.set(kind, h)
  return () => {
    if (handlers.get(kind) === h) handlers.delete(kind)
  }
}

function sub(scope: string): void {
  const h = handlers.get(kindOf(scope))
  h?.subscribed?.(scope)
  const since = h?.since?.(keyOf(scope))
  wsManager.send(ENDPOINT, since ? { op: 'sub', scope, since } : { op: 'sub', scope })
}

/** Holds `scope` until the returned release; the first hold subscribes, the last unsubscribes. */
export function holdScope(scope: string): () => void {
  const count = holds.get(scope) ?? 0
  holds.set(scope, count + 1)
  if (count === 0 && ready) sub(scope)
  let released = false
  return () => {
    if (released) return
    released = true
    const left = (holds.get(scope) ?? 0) - 1
    if (left > 0) {
      holds.set(scope, left)
      return
    }
    // A dropped scope is no longer held; its holders' releases are no-ops.
    if (!holds.delete(scope)) return
    if (ready) wsManager.send(ENDPOINT, { op: 'unsub', scope })
  }
}

/** Stops holding a scope the daemon will not serve, so no reconnect asks again. */
function dropScope(scope: string, why: unknown): void {
  report(why, { action: `Stream ${scope}` })
  if (holds.delete(scope) && ready) wsManager.send(ENDPOINT, { op: 'unsub', scope })
}

function onReady(next: string): void {
  ready = true
  if (next !== epoch) {
    epoch = next
    for (const h of handlers.values()) h.epoch?.(next)
  }
  for (const scope of holds.keys()) sub(scope)
}

function onFrame(f: Frame): void {
  if (f.t === 'ready') return onReady(f.epoch)
  if (f.t === 'error') return report(new Error(f.problem.message), { action: 'Stream' })
  if (!holds.has(f.scope)) return
  handlers.get(kindOf(f.scope))?.frame(f)
  if (f.t !== 'closed') return
  if (f.reason === 'resync') sub(f.scope)
  else dropScope(f.scope, new Error(f.problem?.message ?? `${f.scope} was ${f.reason}`))
}

function onMessage(raw: unknown): void {
  // The manager's own note that the socket came back; the ready frame that follows says the rest.
  if ((raw as { reconnected?: unknown } | null)?.reconnected === true) return
  let f: Frame
  try {
    f = decodeFrame(raw)
  } catch (err) {
    const scope = err instanceof FrameError ? err.scope : undefined
    if (scope && holds.has(scope)) dropScope(scope, err)
    else if (!scope) report(err, { action: 'Stream' })
    return
  }
  onFrame(f)
}

/** Opens the one stream; returns the stop. Called once, at boot. */
export function startStream(): () => void {
  const stopDrops = wsManager.onDrop(ENDPOINT, () => {
    ready = false
  })
  const stop = wsManager.subscribe(ENDPOINT, onMessage)
  return () => {
    ready = false
    stopDrops()
    stop()
  }
}
