import { useSidebarStore } from '@/lib/store/sidebar'
import { resolveHomeRowScope, useHomeTreeStore } from '@/lib/store/home-tree'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import type { PaneGroup } from '@/features/panes/types/pane'

/**
 * Which PROJECT a chat belongs to — the derivation the pane store deliberately
 * never performs.
 *
 * A view's project is a TAG (`PaneSlice.viewProjects`), written when the view
 * is minted, precisely because this walk — chat → workspace → repo → project —
 * is not always answerable: right after a reload the owning workspace store is
 * not mounted, and an empty stage holds no chat at all. So this is used in
 * exactly two places, neither of them a render path (trap 2):
 *
 *   1. ONCE at hydrate, to file each persisted view (§8);
 *   2. in the drop refusal (`openChatIntoPane`), where an unresolvable chat is
 *      ALLOWED through rather than refused — the geometry already makes a
 *      cross-project drop nearly unreachable, and a refusal on a "don't know"
 *      would break ordinary drops the moment the sidebar is a frame behind.
 *
 * Null means nothing loaded right now can name a project for this chat.
 */
export function resolveChatProjectId(chatId: string, workspaceHint?: string | null): string | null {
  // Project home first, same order every other resolver in the app uses: a
  // home row rides no repo, so `repos` below can never see it, and one repo
  // can falsely claim it (see `resolveHomeRowScope`'s own doc).
  const home = resolveHomeRowScope(chatId)
  if (home?.kind === 'chat') return home.projectId

  const repos = useSidebarStore.getState().repos
  for (const repo of repos) {
    if (repo.projectId && repo.chats?.some((c) => c.id === chatId)) return repo.projectId
  }

  const wsId = resolveChatWorkspaceId(chatId, workspaceHint)
  return wsId ? resolveWorkspaceProjectId(wsId) : null
}

/**
 * `wsId`'s owning project: its repo's, or — for project home, which rides no
 * repo at all and is therefore invisible to `repos` — the project whose home
 * workspace it is, asked the same way `resolveHomeRowScope` asks.
 */
export function resolveWorkspaceProjectId(wsId: string): string | null {
  for (const repo of useSidebarStore.getState().repos) {
    if (!repo.projectId) continue
    if (repo.defaultWorkspaceId === wsId) return repo.projectId
    if (repo.workspaces.some((w) => w.id === wsId)) return repo.projectId
  }
  for (const projectId of Object.keys(useHomeTreeStore.getState().trees)) {
    if (getHomeWorkspaceId(projectId) === wsId) return projectId
  }
  return null
}

/**
 * The project a whole VIEW resolves to, from the chats its panes hold — the
 * hydrate-time half of §8, run once per view and never per frame.
 *
 * First answer wins: law 4 means a view's panes cannot legitimately disagree,
 * and a persisted layout from before the partition existed could, in which
 * case any of its members is a better guess than refusing to file it at all.
 */
export function resolveViewProjectId(
  panes: readonly PaneGroup[],
  resolve: (chatId: string) => string | null = resolveChatProjectId,
): string | null {
  for (const pane of panes) {
    if (!pane.chatId) continue
    const projectId = resolve(pane.chatId)
    if (projectId) return projectId
  }
  return null
}
