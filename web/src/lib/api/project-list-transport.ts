import { apiFetch } from '@/lib/api'
import { hostsRegistered } from '@/features/hosts/lib/host-registration'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST } from '@/features/hosts/types'
import { qualify } from '@/lib/host-path'
import type { ProjectListTransport } from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { subscribeGuarded } from '@/lib/ws/subscribe-guarded'

// A paired host's list merges in whenever it answers, so nothing waits on it; the bound
// only keeps a black-holed host from holding a request for the proxy's five minutes.
const REMOTE_READ_TIMEOUT_MS = 5_000

// The read is the retry: a host that does not answer keeps its rows until the next one.
const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

type Row = Omit<Project, 'hostId'>

async function read(hostId: string): Promise<Project[]> {
  if (hostId === LOCAL_HOST.id) {
    if (!hasLocalDaemon()) return []
    return tagged(await apiFetch<Row[]>('/v0/projects', undefined, undefined, hostId), hostId)
  }
  await hostsRegistered()
  const abort = new AbortController()
  const timer = setTimeout(() => abort.abort(), REMOTE_READ_TIMEOUT_MS)
  try {
    return tagged(
      await apiFetch<Row[]>('/v0/projects', { signal: abort.signal }, ONCE, hostId),
      hostId,
    )
  } finally {
    clearTimeout(timer)
  }
}

function tagged(rows: Row[] | undefined, hostId: string): Project[] {
  return (rows ?? []).map((row) => ({ ...row, hostId }))
}

/** The project list's reads and stream, over the app's transport. */
export const projectListTransport: ProjectListTransport = {
  read,
  // Called from a host's status change: a channel that cannot open must not throw into it.
  subscribe: (hostId, onFrame) =>
    hostId === LOCAL_HOST.id && !hasLocalDaemon()
      ? () => {}
      : subscribeGuarded(() => qualify(hostId, '/v0/projects'), onFrame),
}
