import { nanoid } from 'nanoid'
import type { WorkspacePayload } from '@/lib/types'

const INITIAL_WORKSPACES: WorkspacePayload[] = [
  { id: 'ws3', projectId: 'proj1', repoId: 'crowbar', branch: 'feature/app-design' },
  { id: 'ws2', projectId: 'proj1', repoId: 'crowbar', branch: 'feature/api-backend' },
  { id: 'ws1', projectId: 'proj1', repoId: 'crowbar', branch: 'enhancement/scaffold' },
  { id: 'qc1', projectId: 'proj1', repoId: 'quiver-core', branch: 'develop' },
  { id: 'qd1', projectId: 'proj1', repoId: 'quiver-desktop', branch: 'develop' },
  { id: 'qd2', projectId: 'proj1', repoId: 'quiver-desktop', branch: 'feature/quiver-shell' },
]

const store = new Map<string, WorkspacePayload>(INITIAL_WORKSPACES.map((ws) => [ws.id, ws]))

export function getMockWorkspace(wsId: string): WorkspacePayload | undefined {
  return store.get(wsId)
}

export function createMockWorkspace(repoId: string, branch: string): WorkspacePayload {
  const id = nanoid()
  const ws: WorkspacePayload = { id, projectId: '', repoId, branch }
  store.set(id, ws)
  return ws
}
