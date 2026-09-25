import type { Repo } from '@/lib/store/sidebar'
import { chatIconIndex, type ChatIconFields } from './rows-from-repo'

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
