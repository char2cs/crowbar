import { apiFetch } from '@/lib/api/fetch'
import { getOwningChatId } from '@/lib/workspace-scope'
import { chatBase, isHomeWorkspace } from '@/lib/workspace-scope-url'

export interface BlameEntry {
  lineNumber: number
  commitHash: string
  author: string
  email: string
  date: string
  commitMessage: string
}

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
