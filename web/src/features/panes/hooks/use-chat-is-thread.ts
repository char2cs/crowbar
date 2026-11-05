import { selectHomeTrees, selectRepos, useSelect } from '@/core/entities/select'
import type { Chat, Repo } from '@/core/entities/repo-types'
import type { HomeTree } from '@/core/entities/home-rows'

/** Whether the tree positively names `chatId` a thread (owns no worktree; `workspaceId` is its
 *  parent's, so it can't decide). Unseen in `repos` → false, so branch chrome doesn't blink while
 *  chats load; a project-home chat can never own one → true. */
export function chatIsThreadIn(
  repos: readonly Repo[],
  chatId: string | null,
  homeTrees: Record<string, HomeTree> = {},
): boolean {
  if (!chatId) return false
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat) return !ownsWorktree(repo, chat)
  }
  return chatIsInAnyHomeTree(homeTrees, chatId)
}

// Not resolveHomeRowScope: it fails closed while the home workspace id is unresolved, but a
// home chat is a thread regardless.
function chatIsInAnyHomeTree(homeTrees: Record<string, HomeTree>, chatId: string): boolean {
  for (const projectId of Object.keys(homeTrees)) {
    if (homeTrees[projectId].chats.some((c) => c.id === chatId)) return true
  }
  return false
}

// The same owner authorities `deriveRows` reads: the chat's own claim, the repo home's
// owningChatId (home is never in `repo.workspaces`), and the Workspace record's join.
function ownsWorktree(repo: Repo, chat: Chat): boolean {
  if (chat.ownsWorktree) return true
  if (repo.defaultOwningChatId && repo.defaultOwningChatId === chat.id) return true
  return repo.workspaces.some((w) => w.owningChatId === chat.id)
}

/** {@link chatIsThreadIn} in the render path, over the repo chats and the
 *  project-home chats (home rides no repo, so it is never among the former). */
export function useChatIsThread(chatId: string | null): boolean {
  const repos = useSelect(selectRepos)
  const homeTrees = useSelect(selectHomeTrees)
  return chatIsThreadIn(repos, chatId, homeTrees)
}
