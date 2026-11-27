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

export async function commitFiles(
  wsId: string,
  message: string,
  paths: string[],
  unstage: string[],
): Promise<void> {
  const [subject, ...body] = message.trim().split(/\n\s*\n/)
  const post = (action: string, payload: object) =>
    apiFetch(`${gitBaseForWorkspace(wsId)}/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  await post('stage', { paths })
  if (unstage.length > 0) await post('unstage', { paths: unstage })
  await post('commit', { subject: subject.trim(), body: body.join('\n\n').trim() })
}
