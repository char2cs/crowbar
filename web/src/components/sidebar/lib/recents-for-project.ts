import {
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
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
 * A workspace nobody has opened this session has no working/dormant chats to
 * show.
 *
 * Task 26: panes/dormantArrangements are WINDOW-level now (one flat store
 * for the whole app, not one per workspace — see window-pane-store.ts), so
 * this no longer aggregates N separate per-workspace stores' OWN pane trees.
 * `ROOT_PANE_ID`/`BOTTOM_PANE_ID` are no longer duplicated per workspace
 * either (there is exactly one pane store), so the old workspace-qualified
 * id (`${wsId}:${id}`) this function used to mint to dodge that collision is
 * gone — every pane/dormant-arrangement id is already globally unique.
 * What's left to do here is project SCOPING: this project's chats are
 * spread across its own workspaces' `agentChats` (still per-workspace — a
 * chat "belongs" to whichever workspace store's `agentChats.chats` names
 * it), so a single project-wide chat-id set and merged `working` map are
 * built from just `projectWsIds`, and `deriveRecentsEntries` is called ONCE
 * against the one pane store's panes/dormantArrangements, filtered to that
 * set — a pane or dormant arrangement holding some OTHER project's chat is
 * excluded rather than resolved against the wrong store (there is no
 * "wrong store" to resolve against any more, but a wrong PROJECT is still a
 * real thing to guard against; two panes on screen can legitimately belong
 * to different projects' workspaces at once).
 */
export function recentsForProject(repos: readonly Repo[], projectId: string): RecentsBandEntry[] {
  const projectWsIds = getAllActiveWorkspaceIds().filter((wsId) =>
    workspaceIdsForProject(repos, projectId).includes(wsId),
  )

  // chatId -> the workspace whose store owns it (still per-workspace state —
  // AgentChatsSlice did not move in Task 26). Every RecentsBandEntry needs
  // this to render (recents-band.tsx resolves a chat's live data by it).
  const chatWorkspace = new Map<string, string>()
  const working: Record<string, boolean> = {}
  for (const wsId of projectWsIds) {
    const { agentChats } = getOrCreateWorkspaceStore(wsId).getState()
    for (const chat of agentChats.chats) chatWorkspace.set(chat.id, wsId)
    Object.assign(working, agentChats.working)
  }

  const { panes, dormantArrangements } = windowPaneStore.getState()
  const projectPanes = Object.values(panes).filter(
    (p) => p.chatId != null && chatWorkspace.has(p.chatId),
  )
  const projectDormant = dormantArrangements.filter((e) =>
    e.chatIds.some((id) => chatWorkspace.has(id)),
  )

  return deriveRecentsEntries(projectPanes, working, projectDormant).map((entry) => ({
    ...entry,
    // Ids are already globally unique (one pane store, real chat/nanoid ids)
    // — no more workspace-qualification needed, so localId is just id.
    localId: entry.id,
    workspaceId: chatWorkspace.get(entry.chatIds[0]) ?? '',
  }))
}
