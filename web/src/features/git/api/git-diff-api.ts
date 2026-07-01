import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { GitDiff } from '../types/git-types'
import { gitDiffCache } from '../utils/git-diff-cache'

// All diff reads go through the workspace-scoped backend routes
// GET /v0/workspaces/:wsId/git/diff (?path= / ?staged=) and
// GET /v0/workspaces/:wsId/git/commit-diff (?sha=). The first argument is the
// workspace id (the app threads rootFolderPath, which is the wsId, as the
// "repo" handle).

interface MultiFileDiffCacheEntry {
  diffs: GitDiff[]
  timestamp: number
}

interface CommitDiffResponse {
  files: GitDiff[]
}

const MULTI_FILE_DIFF_CACHE_TTL = 30_000
const commitDiffCache = new Map<string, MultiFileDiffCacheEntry>()

const getMultiFileDiffCacheEntry = (
  cache: Map<string, MultiFileDiffCacheEntry>,
  key: string,
): GitDiff[] | null => {
  const entry = cache.get(key)
  if (!entry) return null
  if (Date.now() - entry.timestamp > MULTI_FILE_DIFF_CACHE_TTL) {
    cache.delete(key)
    return null
  }
  return entry.diffs
}

const setMultiFileDiffCacheEntry = (
  cache: Map<string, MultiFileDiffCacheEntry>,
  key: string,
  diffs: GitDiff[],
): void => {
  cache.set(key, { diffs, timestamp: Date.now() })
}

function diffBase(wsId: string): string {
  return `${workspaceBase(wsId)}/git/diff`
}

// Working-tree diff for a single file. Returns the one matching file diff (the
// backend filters by ?path), or null when the file has no changes.
export const getFileDiff = async (
  wsId: string,
  filePath: string,
  staged: boolean = false,
  content?: string,
): Promise<GitDiff | null> => {
  const cached = gitDiffCache.get(wsId, filePath, staged, content)
  if (cached) return cached

  try {
    const query = `?path=${encodeURIComponent(filePath)}&staged=${staged}`
    const diffs = await apiFetch<GitDiff[]>(`${diffBase(wsId)}${query}`)
    const diff = diffs.find((d) => d.file_path === filePath) ?? diffs[0] ?? null
    if (diff) gitDiffCache.set(wsId, filePath, staged, diff, content)
    return diff
  } catch (error) {
    console.error('Failed to get file diff:', error)
    return null
  }
}

// The backend has no content-vs-working diff; fall back to the on-disk diff
// (staged or working tree) so the diff view still renders.
export const getFileDiffAgainstContent = async (
  wsId: string,
  filePath: string,
  _content: string,
  base: 'head' | 'index' = 'head',
): Promise<GitDiff | null> => {
  return getFileDiff(wsId, filePath, base === 'index')
}

export const getCommitDiff = async (
  wsId: string,
  commitHash: string,
): Promise<GitDiff[] | null> => {
  const cacheKey = `${wsId}:${commitHash}`
  const cached = getMultiFileDiffCacheEntry(commitDiffCache, cacheKey)
  if (cached) return cached

  try {
    const res = await apiFetch<CommitDiffResponse>(
      `${workspaceBase(wsId)}/git/commit-diff?sha=${encodeURIComponent(commitHash)}`,
    )
    const diffs = res.files ?? []
    setMultiFileDiffCacheEntry(commitDiffCache, cacheKey, diffs)
    return diffs
  } catch (error) {
    console.error('Failed to get commit diff:', error)
    return null
  }
}

// Ref-range and stash diffs are not yet exposed by the backend; return null so
// callers degrade gracefully rather than throw.
export const getRefDiff = async (
  _wsId: string,
  _baseRef: string,
  _targetRef: string,
): Promise<GitDiff[] | null> => {
  return null
}

export const getStashDiff = async (
  _wsId: string,
  _stashIndex: number,
): Promise<GitDiff[] | null> => {
  return null
}
