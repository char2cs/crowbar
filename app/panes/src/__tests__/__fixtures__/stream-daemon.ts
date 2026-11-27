interface Request {
  op: string
  scope: string
}

interface StreamDaemon {
  sent: Request[]
  answer: (req: Request) => unknown[]
  deliver: (frame: unknown) => void
  drop: () => void
}

const synced = (req: Request) => (req.op === 'sub' ? [{ t: 'synced', scope: req.scope }] : [])
const syncedButProjects = (req: Request) => (req.scope === 'projects' ? [] : synced(req))
const answerOf = (hostId: string) => (hostId === 'local' ? synced : syncedButProjects)

const daemons = new Map<string, StreamDaemon>()

export function streamDaemonFor(hostId: string): StreamDaemon {
  let daemon = daemons.get(hostId)
  if (!daemon) {
    daemon = { sent: [], answer: answerOf(hostId), deliver: () => {}, drop: () => {} }
    daemons.set(hostId, daemon)
  }
  return daemon
}

export const streamDaemon = streamDaemonFor('local')

export function reconnectStream(hostId = 'local', epoch = 'test'): void {
  const daemon = streamDaemonFor(hostId)
  daemon.drop()
  daemon.deliver({ t: 'ready', epoch })
}

export function heldScopes(prefix: string, hostId = 'local'): string[] {
  const held = new Set<string>()
  for (const { op, scope } of streamDaemonFor(hostId).sent) {
    if (!scope.startsWith(prefix)) continue
    if (op === 'sub') held.add(scope)
    else held.delete(scope)
  }
  return [...held]
}

export function resetStreamDaemon(): void {
  for (const [hostId, daemon] of daemons) {
    daemon.sent.length = 0
    daemon.answer = answerOf(hostId)
  }
}
