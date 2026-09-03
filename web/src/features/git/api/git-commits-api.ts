import { apiFetch } from '@/lib/api'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitCommit } from '../types/git-types'

// Commit the staged changes. The first line is the subject; anything after a
// blank line is the body, matching the backend's {subject, body} contract.
export const commitChanges = async (wsId: string, message: string): Promise<boolean> => {
  const [subject, ...rest] = message.split('\n\n')
  try {
    await apiFetch(`${gitBaseForWorkspace(wsId)}/commit`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ subject: subject.trim(), body: rest.join('\n\n').trim() }),
    })
    return true
  } catch (error) {
    console.error('git commit failed:', error)
    return false
  }
}

export const getGitLog = async (wsId: string, limit = 50, skip = 0): Promise<GitCommit[]> => {
  try {
    return await apiFetch<GitCommit[]>(`${gitBaseForWorkspace(wsId)}/log?limit=${limit}&skip=${skip}`)
  } catch {
    return []
  }
}
