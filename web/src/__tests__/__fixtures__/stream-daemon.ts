interface Request {
  op: string
  scope: string
}

/** The daemon behind `/v0/stream` in every test that keeps setup's socket manager. */
interface StreamDaemon {
  /** Every request the client sent, in order. */
  sent: Request[]
  /** The frames a request is answered with; by default a sub's synced, at once. */
  answer: (req: Request) => unknown[]
  /** Hands frames to the client, as the daemon writing them. */
  deliver: (frame: unknown) => void
  /** Drops the socket; `reconnect` opens the next one. */
  drop: () => void
}

const synced = (req: Request) => (req.op === 'sub' ? [{ t: 'synced', scope: req.scope }] : [])

export const streamDaemon: StreamDaemon = {
  sent: [],
  answer: synced,
  deliver: () => {},
  drop: () => {},
}

/** A dropped socket and the next one: its ready re-sends every held sub. */
export function reconnectStream(): void {
  streamDaemon.drop()
  streamDaemon.deliver({ t: 'ready', epoch: 'test' })
}

/** The scopes the daemon serves this client now, by their kind's prefix. */
export function heldScopes(prefix: string): string[] {
  const held = new Set<string>()
  for (const { op, scope } of streamDaemon.sent) {
    if (!scope.startsWith(prefix)) continue
    if (op === 'sub') held.add(scope)
    else held.delete(scope)
  }
  return [...held]
}

/** Back to a daemon that answers every sub with its synced. */
export function resetStreamDaemon(): void {
  streamDaemon.sent.length = 0
  streamDaemon.answer = synced
}

/** Each chat feed's subscribers at setup's fake manager. */
export const feeds = new Map<string, number>()

/** Whether anything subscribes to the chat feed at `endpoint`. */
export function feedHeld(endpoint: string): boolean {
  return (feeds.get(endpoint) ?? 0) > 0
}
