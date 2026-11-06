import { apiFetch } from '@/lib/api/fetch'
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

/** Stages every change, then commits it; a message's first paragraph is the subject. Throws the
 *  daemon's reason. */
export async function commitAll(wsId: string, message: string): Promise<void> {
  const [subject, ...body] = message.trim().split(/\n\s*\n/)
  const post = (action: string, payload: object) =>
    apiFetch(`${gitBaseForWorkspace(wsId)}/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  await post('stage', { paths: ['.'] })
  await post('commit', { subject: subject.trim(), body: body.join('\n\n').trim() })
}
