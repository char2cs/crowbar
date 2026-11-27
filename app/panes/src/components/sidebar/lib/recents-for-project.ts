import type { Repo } from '@/core/entities/repo-types'
import { chatIconIndex, type ChatIconFields } from './chat-icon-index'

const iconCache = new WeakMap<readonly Repo[], Map<string, ChatIconFields>>()

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
