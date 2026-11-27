import { streamDaemonFor } from '@/__tests__/__fixtures__/stream-daemon'
import { chatEntity } from '@/__tests__/__fixtures__/sidebar-entities'

let v = 0

interface Running {
  title?: string
  workspaceId?: string
  viewers?: number
  hostId?: string
}

export function runChat(id: string, over: Running = {}): void {
  const { title = id, workspaceId = 'ws-1', viewers = 0, hostId = 'local' } = over
  const daemon = streamDaemonFor(hostId)
  const data = chatEntity({ id, title, workspaceId, liveRunnerId: `run-${id}` })
  daemon.deliver({ t: 'entity', scope: 'projects', kind: 'chat', id, v: ++v, data })
  if (viewers > 0) {
    const presence = { viewers }
    daemon.deliver({ t: 'entity', scope: 'projects', kind: 'presence', id, v: ++v, data: presence })
  }
  if (hostId !== 'local') daemon.deliver({ t: 'synced', scope: 'projects' })
}
