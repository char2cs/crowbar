import { apiFetch } from '@/lib/api'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitStash } from '../types/git-types'

interface StashDTO {
  id: string
  message: string
  date: string
  filesChanged: number
}

const base = gitBaseForWorkspace

export const getStashes = async (wsId: string): Promise<GitStash[]> => {
  try {
    const stashes = await apiFetch<StashDTO[]>(`${base(wsId)}/stashes`)
    return stashes.map((s, index) => ({ index, message: s.message, date: s.date }))
  } catch {
    return []
  }
}
