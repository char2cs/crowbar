import {
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { deriveRecentsEntries } from './recents-entries'
import type { Repo } from '@/lib/store/sidebar'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'

// Re-exported under its original name for existing importers/tests — the
// canonical declaration lives in recents-band.tsx (the render contract every
// caller of this module ultimately feeds), so the two can't drift apart.
export type { RecentsBandEntry as ProjectRecentsEntry }

/**
 * Every workspace id under `projectId`'s repos — the repo-home id
 * (`defaultWorkspaceId`, not itself a `Workspace` row) included, PLUS the
 * project's own home-workspace id if resolved.
 *
 * Project home (`/ide/$projectId/home`, the landing surface every project
 * switch goes to) is a real, store-backed `WorkspaceView` but deliberately
 * carries no tree row and sits outside `Repo.workspaces` entirely
 * (`workspace-host.tsx`: "Home is a project-level concept, not a repo
 * workspace"). Omitting it here would silently exclude any chat opened on
 * project home from that project's own Recents band.
 */
export function workspaceIdsForProject(repos: readonly Repo[], projectId: string): string[] {
  const ids: string[] = []
  for (const repo of repos) {
    if (repo.projectId !== projectId) continue
    if (repo.defaultWorkspaceId) ids.push(repo.defaultWorkspaceId)
    for (const ws of repo.workspaces) ids.push(ws.id)
  }
  const homeId = getHomeWorkspaceId(projectId)
  if (homeId) ids.push(homeId)
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
 *
 * Every produced entry's `.id` is workspace-qualified (`${wsId}:${id}`):
 * `deriveRecentsEntries` keys an entry by a pane id or a dormant
 * arrangement's id, and `ROOT_PANE_ID`/`BOTTOM_PANE_ID` are module-level
 * constants — identical across every workspace store. Two retained
 * workspaces under one project each holding a chat in their root pane would
 * otherwise both produce an entry literally id `'root-pane'`, a guaranteed
 * React-key/`data-testid` collision. `localId` carries the original,
 * workspace-scoped id through for any caller (e.g.
 * `paneActions.forgetDormantArrangement`) that needs to match it against the
 * owning store's own real stored state.
 */
export function recentsForProject(repos: readonly Repo[], projectId: string): RecentsBandEntry[] {
  const projectWsIds = new Set(workspaceIdsForProject(repos, projectId))
  const entries: RecentsBandEntry[] = []
  for (const wsId of getAllActiveWorkspaceIds()) {
    if (!projectWsIds.has(wsId)) continue
    const state = getOrCreateWorkspaceStore(wsId).getState()
    const perWorkspace = deriveRecentsEntries(
      Object.values(state.panes),
      state.agentChats.working,
      state.dormantArrangements,
    )
    for (const entry of perWorkspace) {
      entries.push({ ...entry, id: `${wsId}:${entry.id}`, localId: entry.id, workspaceId: wsId })
    }
  }
  return entries
}
