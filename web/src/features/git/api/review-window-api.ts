import { apiFetch, apiFetchRaw } from '@/lib/api/fetch'
import { reviewBaseForWorkspace as reviewBase } from '@/lib/workspace-scope-url'

// The windowed branch-review client, each read bounded by what the reader sees: outline (hunk
// geometry, no content), one file's patch, contents for expansion, and server-side search (the
// client holds only a window). JSON envelopes like review-api.ts, except the patch.

/** Narrows a windowed read from the whole branch (the default when absent) to one commit. */
const SCOPE_COMMIT_PARAM = 'sha'

/** Which diff a read is about: no `commit` means the branch against its parent; a commit narrows
 *  every read to that commit, so history reuses the review surface wholesale. */
export interface DiffScope {
  wsId: string
  commit?: string
}

function scopeParams(scope: DiffScope, init?: Record<string, string>): URLSearchParams {
  const params = new URLSearchParams(init)
  if (scope.commit) params.set(SCOPE_COMMIT_PARAM, scope.commit)
  return params
}

// ── Outline ─────────────────────────────────────────────────────────

/** One `@@` header reduced to its four numbers (gitdomain.HunkShape).
 *  The line counts include context lines, so they describe exactly how tall the
 *  hunk renders — which is the point: scroll space is reserved from them. */
export interface HunkShape {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
}

/** One file's geometry (gitdomain.FileOutline). A rename's `path` is the new name (what
 *  /review/patch takes); a binary has no hunks. `isPartial`: hunks ran past the server cap, so
 *  geometry is a lower bound — top it up from the summary's ± counts. */
export interface FileOutline {
  path: string
  oldPath?: string
  hunks: HunkShape[]
  isPartial: boolean
  isBinary: boolean
}

/** Wire shape of FileOutline. Go marshals a nil slice as `null`, which is what a
 *  binary file's empty Hunks produces, so the null is normalised away here
 *  rather than at every consumer that maps over hunks. */
interface WireFileOutline extends Omit<FileOutline, 'hunks'> {
  hunks: HunkShape[] | null
}

/** GET the hunk geometry of the whole branch diff — O(hunks), never O(lines).
 *  A million-line branch is described in about a megabyte, which is what lets
 *  the review surface lay out every file before fetching a single patch. */
export async function getReviewOutline(scope: DiffScope): Promise<FileOutline[]> {
  const query = scopeParams(scope).toString()
  const raw = await apiFetch<{ files: WireFileOutline[] }>(
    `${reviewBase(scope.wsId)}/outline${query ? `?${query}` : ''}`,
  )
  return (raw.files ?? []).map((f) => ({ ...f, hunks: f.hunks ?? [] }))
}

// ── Patch ───────────────────────────────────────────────────────────

/** Reports a capped patch was cut short. A leading header, not a trailer, because `fetch()`
 *  exposes no trailers; the server buffers the (cap-bounded) body to answer up front. */
export const DIFF_TRUNCATED_HEADER = 'X-Crowbar-Diff-Truncated'

export interface ReviewPatch {
  patch: string
  /** True only when a CAPPED request was cut short. An uncapped patch streams and
   *  reports nothing, because an uncapped patch cannot be truncated. */
  truncated: boolean
}

/** One file's patch as text/plain with the truncation flag in a header, so apiFetchRaw, not
 *  apiFetch. `maxLines` omitted takes the server cap; `<= 0` is unlimited ("Show all"), streamed
 *  and never truncated. */
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

// ── File contents ───────────────────────────────────────────────────

/** Both sides of one file in full (gitdomain.ReviewFileSides); null where the
 *  file does not exist on that side. */
interface ReviewFileSides {
  old: string | null
  new: string | null
}

/** One file's full old and new text, fetched only when the reader expands unchanged lines; the
 *  daemon refuses a file past its size ceiling (422). `oldPath` is sent only for a rename. */
export async function getReviewFileSides(
  scope: DiffScope,
  path: string,
  oldPath?: string,
): Promise<ReviewFileSides> {
  const params = scopeParams(scope, { path })
  if (oldPath !== undefined && oldPath !== path) params.set('oldPath', oldPath)
  return apiFetch<ReviewFileSides>(`${reviewBase(scope.wsId)}/contents?${params.toString()}`)
}

// ── Search ──────────────────────────────────────────────────────────

/** One diff-content match. `lineNumber` is in the file on `side`: removed → old, added and
 *  context → new. `preview` drops the diff prefix and is capped server-side, so in a minified
 *  file it may end before the match. */
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
  /** True when the server's limit cut the results short — surface it, never a silent cut. */
  truncated: boolean
}

/** Find-in-diff. An empty query answers empty without scanning, so it is safe per keystroke; an
 *  invalid regex is a 400, an ApiError the caller renders inline. */
export async function searchReviewDiff(
  scope: DiffScope,
  q: string,
  opts?: SearchDiffOptions,
): Promise<SearchDiffResult> {
  const params = scopeParams(scope, { q })
  if (opts?.regex) params.set('regex', 'true')
  // The wire parameter is `case`, not `caseSensitive`.
  if (opts?.caseSensitive) params.set('case', 'true')
  if (opts?.limit !== undefined) params.set('limit', String(opts.limit))
  const raw = await apiFetch<{ hits: SearchHit[] | null; truncated: boolean }>(
    `${reviewBase(scope.wsId)}/search?${params.toString()}`,
  )
  return { hits: raw.hits ?? [], truncated: raw.truncated ?? false }
}
