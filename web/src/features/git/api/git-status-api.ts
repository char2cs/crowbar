import { apiFetch } from '@/lib/api/fetch'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { GitHunk, GitStatus } from '../types/git-types'
import { report } from '@/lib/errors/report'

async function gitPost(
  wsId: string,
  action: string,
  body: Record<string, unknown>,
): Promise<boolean> {
  try {
    await apiFetch(`${gitBaseForWorkspace(wsId)}/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    return true
  } catch (error) {
    console.error(`git ${action} failed:`, error)
    report(error, { action: `git ${action} failed` })
    return false
  }
}

function hunkIdOf(hunk: GitHunk): string | undefined {
  const h = hunk as GitHunk & { hunkId?: string; id?: string }
  return h.hunkId ?? h.id
}

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

export const getGitStatus = async (wsId: string): Promise<GitStatus | null> =>
  (await readGitStatus(wsId)).status

export const unstagePaths = (wsId: string, paths: string[]): Promise<boolean> =>
  gitPost(wsId, 'unstage', { paths })

// Hunk-level staging when the diff carries a hunkId; otherwise fall back to
// staging the whole file so the action still has an effect.
export const stageHunk = (wsId: string, hunk: GitHunk): Promise<boolean> => {
  const hunkId = hunkIdOf(hunk)
  return hunkId
    ? gitPost(wsId, 'stage-hunk', { path: hunk.file_path, hunkId })
    : gitPost(wsId, 'stage', { paths: [hunk.file_path] })
}

export const unstageHunk = (wsId: string, hunk: GitHunk): Promise<boolean> => {
  const hunkId = hunkIdOf(hunk)
  return hunkId
    ? gitPost(wsId, 'unstage-hunk', { path: hunk.file_path, hunkId })
    : gitPost(wsId, 'unstage', { paths: [hunk.file_path] })
}
