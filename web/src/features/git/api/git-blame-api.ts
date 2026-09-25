import { apiFetch } from '@/lib/api'
import { getOwningChatId } from '@/lib/workspace-scope'
import { chatBase, isHomeWorkspace } from '@/lib/workspace-scope-url'

/** One line's last-changing commit, as the daemon's GET /blame answers it. */
export interface BlameEntry {
  /** 1-based. */
  lineNumber: number
  commitHash: string
  author: string
  email: string
  /** RFC 3339. */
  date: string
  commitMessage: string
}

/**
 * Blame a workspace-relative file (GET /v0/chats/:chatId/blame?path=).
 * Resolves null when the workspace cannot be addressed yet (home workspace,
 * owning chat not recorded).
 */
export async function getBlame(
  wsId: string,
  path: string,
  signal?: AbortSignal,
): Promise<BlameEntry[] | null> {
  const chatId = isHomeWorkspace(wsId) ? null : getOwningChatId(wsId)
  if (!chatId) return null
  return apiFetch<BlameEntry[]>(`${chatBase(chatId)}/blame?path=${encodeURIComponent(path)}`, {
    signal,
  })
}
