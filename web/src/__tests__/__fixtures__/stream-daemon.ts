interface Request {
  op: string
  scope: string
}

/** One host's daemon behind `/v0/stream` in every test that keeps setup's socket manager. */
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

const daemons = new Map<string, StreamDaemon>()

/** `hostId`'s daemon, as the client's socket to that host reaches it. */
export function streamDaemonFor(hostId: string): StreamDaemon {
  let daemon = daemons.get(hostId)
  if (!daemon) {
    daemon = { sent: [], answer: synced, deliver: () => {}, drop: () => {} }
    daemons.set(hostId, daemon)
  }
  return daemon
}

/** The local daemon. */
export const streamDaemon = streamDaemonFor('local')

/** A dropped socket and the next one: its ready re-sends every held sub. */
export function reconnectStream(hostId = 'local', epoch = 'test'): void {
  const daemon = streamDaemonFor(hostId)
  daemon.drop()
  daemon.deliver({ t: 'ready', epoch })
}

/** The scopes `hostId`'s daemon serves this client now, by their kind's prefix. */
export function heldScopes(prefix: string, hostId = 'local'): string[] {
  const held = new Set<string>()
  for (const { op, scope } of streamDaemonFor(hostId).sent) {
    if (!scope.startsWith(prefix)) continue
    if (op === 'sub') held.add(scope)
    else held.delete(scope)
  }
  return [...held]
}

/** Back to daemons that answer every sub with its synced. */
export function resetStreamDaemon(): void {
  for (const daemon of daemons.values()) {
    daemon.sent.length = 0
    daemon.answer = synced
  }
}
