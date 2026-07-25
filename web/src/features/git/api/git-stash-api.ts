import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { GitStash } from '../types/git-types'

interface StashDTO {
  id: string
  message: string
  date: string
  filesChanged: number
}

function base(wsId: string): string {
  return `${workspaceBase(wsId)}/git`
}

export const getStashes = async (wsId: string): Promise<GitStash[]> => {
  try {
    const stashes = await apiFetch<StashDTO[]>(`${base(wsId)}/stashes`)
    return stashes.map((s, index) => ({ index, message: s.message, date: s.date }))
  } catch {
    return []
  }
}
