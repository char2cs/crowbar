import { apiFetch } from '@/lib/api/fetch'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitStatus } from '../types/git-types'

export async function readGitStatus(
  wsId: string,
): Promise<{ status: GitStatus | null; error: string | null }> {
  try {
    const status = await apiFetch<GitStatus>(`${gitBaseForWorkspace(wsId)}/status`)
    return { status: { ...status, files: status.files ?? [] }, error: null }
  } catch (err) {
    return { status: null, error: err instanceof Error ? err.message : String(err) }
  }
}
