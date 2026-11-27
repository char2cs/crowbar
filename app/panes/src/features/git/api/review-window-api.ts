import { apiFetch, apiFetchRaw } from '@/lib/api/fetch'
import { reviewBaseForWorkspace as reviewBase } from '@/lib/workspace-scope-url'

const SCOPE_COMMIT_PARAM = 'sha'

export interface DiffScope {
  wsId: string
  commit?: string
}

function scopeParams(scope: DiffScope, init?: Record<string, string>): URLSearchParams {
  const params = new URLSearchParams(init)
  if (scope.commit) params.set(SCOPE_COMMIT_PARAM, scope.commit)
  return params
}

export interface HunkShape {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
}

export interface FileOutline {
  path: string
  oldPath?: string
  hunks: HunkShape[]
  isPartial: boolean
  isBinary: boolean
}

interface WireFileOutline extends Omit<FileOutline, 'hunks'> {
  hunks: HunkShape[] | null
}

export async function getReviewOutline(scope: DiffScope): Promise<FileOutline[]> {
  const query = scopeParams(scope).toString()
  const raw = await apiFetch<{ files: WireFileOutline[] }>(
    `${reviewBase(scope.wsId)}/outline${query ? `?${query}` : ''}`,
  )
  return (raw.files ?? []).map((f) => ({ ...f, hunks: f.hunks ?? [] }))
}

const DIFF_TRUNCATED_HEADER = 'X-Crowbar-Diff-Truncated'

export interface ReviewPatch {
  patch: string
  truncated: boolean
}

export async function getReviewPatch(
  scope: DiffScope,
  path: string,
  maxLines?: number,
): Promise<ReviewPatch> {
  const params = scopeParams(scope, { path })
  if (maxLines !== undefined) params.set('maxLines', String(maxLines))
  const res = await apiFetchRaw(`${reviewBase(scope.wsId)}/patch?${params.toString()}`)
  const patch = await res.text()
  return { patch, truncated: res.headers.get(DIFF_TRUNCATED_HEADER) === 'true' }
}

interface ReviewFileSides {
  old: string | null
  new: string | null
}

export async function getReviewFileSides(
  scope: DiffScope,
  path: string,
  oldPath?: string,
): Promise<ReviewFileSides> {
  const params = scopeParams(scope, { path })
  if (oldPath !== undefined && oldPath !== path) params.set('oldPath', oldPath)
  return apiFetch<ReviewFileSides>(`${reviewBase(scope.wsId)}/contents?${params.toString()}`)
}

export interface SearchHit {
  path: string
  side: 'old' | 'new'
  lineNumber: number
  preview: string
}

export interface SearchDiffOptions {
  regex?: boolean
  caseSensitive?: boolean
  limit?: number
}

export interface SearchDiffResult {
  hits: SearchHit[]
  truncated: boolean
}

export async function searchReviewDiff(
  scope: DiffScope,
  q: string,
  opts?: SearchDiffOptions,
): Promise<SearchDiffResult> {
  const params = scopeParams(scope, { q })
  if (opts?.regex) params.set('regex', 'true')
  if (opts?.caseSensitive) params.set('case', 'true')
  if (opts?.limit !== undefined) params.set('limit', String(opts.limit))
  const raw = await apiFetch<{ hits: SearchHit[] | null; truncated: boolean }>(
    `${reviewBase(scope.wsId)}/search?${params.toString()}`,
  )
  return { hits: raw.hits ?? [], truncated: raw.truncated ?? false }
}
