import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import type { HomeTree } from '@/lib/store/home-tree'
import type { Repo } from '@/lib/store/sidebar'
import { chatIconIndex, type ChatIconFields } from './rows-from-repo'

export function repoChatWorkspaceId(
  repos: readonly Repo[],
  projectId: string,
  chatId: string,
): string | null {
  for (const repo of repos) {
    if (repo.projectId !== projectId) continue
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat?.workspaceId) return chat.workspaceId
  }
  return null
}

export function homeChatWorkspaceId(
  homeTrees: Readonly<Record<string, HomeTree>>,
  projectId: string,
  chatId: string,
): string | null {
  const home = homeTrees[projectId]?.chats.find((c) => c.id === chatId)
  if (!home) return null
  return home.workspaceId || getHomeWorkspaceId(projectId) || null
}

/**
 * The workspace a band member's chat belongs to. The sidebar's own chat
 * lists answer first — they survive a reload before any workspace store
 * mounts — then the registered stores. `listChats` is repo-scoped, so the
 * chat's own `workspaceId` is used, never the store that happened to carry it.
 */
export function recentsChatWorkspaceId(
  repos: readonly Repo[],
  homeTrees: Readonly<Record<string, HomeTree>>,
  projectId: string,
  chatId: string,
): string {
  return (
    repoChatWorkspaceId(repos, projectId, chatId) ??
    homeChatWorkspaceId(homeTrees, projectId, chatId) ??
    resolveChatWorkspaceId(chatId) ??
    ''
  )
}

const iconCache = new WeakMap<readonly Repo[], Map<string, ChatIconFields>>()

/** The tree's own branch/lock/PR glyph fields for a workspace-owning chat,
 *  shared across every row for one `repos` snapshot. */
export function recentsChatIcon(
  repos: readonly Repo[],
  chatId: string,
): ChatIconFields | undefined {
  let index = iconCache.get(repos)
  if (!index) {
    index = chatIconIndex(repos)
    iconCache.set(repos, index)
  }
  return index.get(chatId)
}
