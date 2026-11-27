import { selectHomeTrees, selectRepos, useSelect } from '@/core/entities/select'
import type { Chat, Repo } from '@/core/entities/repo-types'
import type { HomeTree } from '@/core/entities/home-rows'

function chatIsThreadIn(
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

function chatIsInAnyHomeTree(homeTrees: Record<string, HomeTree>, chatId: string): boolean {
  for (const projectId of Object.keys(homeTrees)) {
    if (homeTrees[projectId].chats.some((c) => c.id === chatId)) return true
  }
  return false
}

function ownsWorktree(repo: Repo, chat: Chat): boolean {
  if (chat.ownsWorktree) return true
  if (repo.defaultOwningChatId && repo.defaultOwningChatId === chat.id) return true
  return repo.workspaces.some((w) => w.owningChatId === chat.id)
}

export function useChatIsThread(chatId: string | null): boolean {
  return useSelect((s) => chatIsThreadIn(selectRepos(s), chatId, selectHomeTrees(s)))
}
