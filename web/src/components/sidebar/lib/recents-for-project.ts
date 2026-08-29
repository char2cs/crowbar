import {
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { deriveRecentsEntries } from './recents-entries'
import type { Repo } from '@/lib/store/sidebar'
import type { RecentsEntry } from '@/features/panes/types/recents-entry'

/** A `RecentsEntry` tagged with the workspace whose store its chats live in —
 *  needed once entries can come from more than one workspace (spec §4:
 *  "Recents is per space", not per workspace). Every chat in one entry
 *  shares a workspace by construction (an entry's chats came from one
 *  workspace's own panes/dormantArrangements), so one tag per entry is
 *  enough. */
export interface ProjectRecentsEntry extends RecentsEntry {
  workspaceId: string
}

/** Every workspace id under `projectId`'s repos — the repo-home id
 *  (`defaultWorkspaceId`, not itself a `Workspace` row) included. */
export function workspaceIdsForProject(repos: readonly Repo[], projectId: string): string[] {
  const ids: string[] = []
  for (const repo of repos) {
    if (repo.projectId !== projectId) continue
    if (repo.defaultWorkspaceId) ids.push(repo.defaultWorkspaceId)
    for (const ws of repo.workspaces) ids.push(ws.id)
  }
  return ids
}

/**
 * Recents entries for every workspace under `projectId`'s repos — spec §4:
 * "Recents is per space for the same reason [no row carries its project]".
 *
 * Only workspaces that ALREADY have a live store — `getAllActiveWorkspaceIds`,
 * populated by `WorkspaceHost`'s active + keep-alive-retained set — are read.
 * A workspace nobody has opened this session has no panes, no working chats
 * and no dormant arrangements to show; calling `getOrCreateWorkspaceStore`
 * for one anyway would register a store WorkspaceHost never mounted and so
 * never destroys, leaking it for the life of the session.
 */
export function recentsForProject(
  repos: readonly Repo[],
  projectId: string,
): ProjectRecentsEntry[] {
  const projectWsIds = new Set(workspaceIdsForProject(repos, projectId))
  const entries: ProjectRecentsEntry[] = []
  for (const wsId of getAllActiveWorkspaceIds()) {
    if (!projectWsIds.has(wsId)) continue
    const state = getOrCreateWorkspaceStore(wsId).getState()
    const perWorkspace = deriveRecentsEntries(
      Object.values(state.panes),
      state.agentChats.working,
      state.dormantArrangements,
    )
    for (const entry of perWorkspace) entries.push({ ...entry, workspaceId: wsId })
  }
  return entries
}
