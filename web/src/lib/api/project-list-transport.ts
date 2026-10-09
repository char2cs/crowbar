import { apiFetch } from '@/lib/api'
import type { ProjectListTransport } from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { wsManager } from '@/lib/ws/manager'

async function read(hostId: string): Promise<Project[]> {
  const rows = await apiFetch<Omit<Project, 'hostId'>[]>(
    '/v0/projects',
    undefined,
    undefined,
    hostId,
  )
  return (rows ?? []).map((row) => ({ ...row, hostId }))
}

/** The project list's reads and stream, over the app's transport. */
export const projectListTransport: ProjectListTransport = {
  read,
  subscribe: (onFrame) => wsManager.subscribe('/v0/projects', onFrame),
}
