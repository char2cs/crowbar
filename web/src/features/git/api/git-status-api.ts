import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { toast } from '@/features/window/stores/toast-store'
import type { GitHunk, GitStatus } from '../types/git-types'

async function gitPost(
  wsId: string,
  action: string,
  body: Record<string, unknown>,
): Promise<boolean> {
  try {
    await apiFetch(`${workspaceBase(wsId)}/git/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    return true
  } catch (error) {
    console.error(`git ${action} failed:`, error)
    toast.error(`git ${action} failed`, error instanceof Error ? error.message : undefined)
    return false
  }
}

function hunkIdOf(hunk: GitHunk): string | undefined {
  const h = hunk as GitHunk & { hunkId?: string; id?: string }
  return h.hunkId ?? h.id
}

export const getGitStatus = async (wsId: string): Promise<GitStatus | null> => {
  try {
    const status = await apiFetch<GitStatus>(`${workspaceBase(wsId)}/git/status`)
    // The backend serialises a clean working tree as `files: null`; normalise
    // here so downstream consumers can rely on `files` being an array.
    return { ...status, files: status.files ?? [] }
  } catch {
    return null
  }
}

export const stageFile = (wsId: string, filePath: string): Promise<boolean> =>
  gitPost(wsId, 'stage', { paths: [filePath] })

export const unstageFile = (wsId: string, filePath: string): Promise<boolean> =>
  gitPost(wsId, 'unstage', { paths: [filePath] })

export const stageAllFiles = (wsId: string): Promise<boolean> =>
  gitPost(wsId, 'stage', { paths: ['.'] })

export const unstageAllFiles = (wsId: string): Promise<boolean> =>
  gitPost(wsId, 'unstage', { paths: ['.'] })

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

export const discardAllChanges = (wsId: string): Promise<boolean> =>
  gitPost(wsId, 'discard', { paths: ['.'] })

export const discardFileChanges = (wsId: string, filePath: string): Promise<boolean> =>
  gitPost(wsId, 'discard', { paths: [filePath] })
