import { apiFetch } from '@/lib/api'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitCommit } from '../types/git-types'

export const getGitLog = async (wsId: string, limit = 50, skip = 0): Promise<GitCommit[]> => {
  try {
    return await apiFetch<GitCommit[]>(
      `${gitBaseForWorkspace(wsId)}/log?limit=${limit}&skip=${skip}`,
    )
  } catch {
    return []
  }
}
