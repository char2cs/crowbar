import { apiFetch } from '@/lib/api/fetch'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitStatus } from '../types/git-types'

/** The worktree's status, or why git could not read it (a moved or deleted repo). */
export async function readGitStatus(
  wsId: string,
): Promise<{ status: GitStatus | null; error: string | null }> {
  try {
    const status = await apiFetch<GitStatus>(`${gitBaseForWorkspace(wsId)}/status`)
    // The backend serialises a clean working tree as `files: null`; normalise
    // here so downstream consumers can rely on `files` being an array.
    return { status: { ...status, files: status.files ?? [] }, error: null }
  } catch (err) {
    return { status: null, error: err instanceof Error ? err.message : String(err) }
  }
}
