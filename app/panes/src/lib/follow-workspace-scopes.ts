import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { splitHostKey } from '@/core/stream/host-scope'
import { forgetWorkspaceScope, recordWorkspaceScope } from '@/lib/workspace-scope'

type Workspaces = EntityState['byKind']['workspace']
type WorkspaceRecord = Workspaces[string]['data']

function record(ws: WorkspaceRecord) {
  const owningChatId = ws.owningChatId || undefined
  recordWorkspaceScope({ projectId: ws.projectId, repoId: ws.repoId, wsId: ws.id, owningChatId })
}

function follow(next: Workspaces, prev: Workspaces) {
  for (const key of Object.keys(prev)) {
    if (!(key in next)) forgetWorkspaceScope(splitHostKey(key).id)
  }
  for (const [key, { data: ws }] of Object.entries(next)) {
    if (prev[key]?.data !== ws && ws.projectId) record(ws)
  }
}

function onChange(s: EntityState, prev: EntityState) {
  if (s.byKind.workspace !== prev.byKind.workspace)
    follow(s.byKind.workspace, prev.byKind.workspace)
}

/** Records each workspace record's scope (project, repo, owning chat) as it arrives and forgets it
 *  when the record goes, so every page can build the chat-scoped URLs of any workspace it shows.
 *  Returns the stop. */
export function followWorkspaceScopes(): () => void {
  follow(entityStore.getState().byKind.workspace, {})
  return entityStore.subscribe(onChange)
}
