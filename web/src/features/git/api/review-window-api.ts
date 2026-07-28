import { apiFetch, apiFetchRaw } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

// The WINDOWED branch-review client: geometry, one file's patch, and search.
//
// It is the counterpart to review-api.ts, whose `getReview` answers the whole
// diff in one composite payload — 158 MB and 1.4M line objects on a 1M-line
// branch. These three routes replace that with reads whose cost is bounded by
// what the reader is actually looking at:
//
//   /review/outline  — every file's hunk geometry and NO content, so the client
//                      can reserve correct scroll space before fetching a byte.
//   /review/patch    — exactly one file's unified patch, text/plain.
//   /review/search   — find-in-diff run server-side, because a client holding
//                      only a window of the patch has nothing left to search.
//
// Conventions follow review-api.ts: workspace-scoped hierarchical URLs and the
// {success,data} envelope that apiFetch unwraps. /review/patch is the exception
// — see getReviewPatch.

function reviewBase(wsId: string): string {
  return `${workspaceBase(wsId)}/review`
}

/** The query parameter that narrows a windowed read from the workspace's whole
 *  branch to a single commit. Absent means the branch — the default and the
 *  surface's reason for existing. */
const SCOPE_COMMIT_PARAM = 'sha'

/** Which diff a windowed read is about.
 *
 *  `commit` empty/undefined means the workspace's branch against its parent —
 *  the GitHub-like review step. A commit narrows every read to that commit
 *  against its parent, which is what lets the history surface reuse the review
 *  surface wholesale instead of keeping a second diff renderer alive for it. */
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

/** One file of the branch diff described by its geometry alone (gitdomain.FileOutline).
 *
 *  A rename carries the NEW name in `path` (the name /review/patch is addressed
 *  with) and the source in `oldPath`. A binary file has no hunks: git emits no
 *  `@@` headers for one.
 *
 *  `isPartial` marks a file whose hunk count ran past the server's per-file cap.
 *  Its geometry is then a LOWER BOUND — the last hunk is not the end of the file
 *  — so a consumer sizing the file from it must top the estimate up from the
 *  summary's ±counts or the file under-reserves space. */
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

/** Response header reporting that a capped patch was cut short.
 *
 *  A LEADING header, deliberately not an HTTP trailer: whether the cap was
 *  reached is only known once the last hunk is written, so a trailer is the
 *  natural HTTP answer — and `fetch()` exposes no trailers, so the one client
 *  this API has could never read it. The server holds a capped body (bounded by
 *  the cap, which is the point of asking for one) to put the answer up front. */
export const DIFF_TRUNCATED_HEADER = 'X-Crowbar-Diff-Truncated'

export interface ReviewPatch {
  patch: string
  /** True only when a CAPPED request was cut short. An uncapped patch streams and
   *  reports nothing, because an uncapped patch cannot be truncated. */
  truncated: boolean
}

/**
 * GET ONE file's unified patch as text/plain.
 *
 * This is the only v0 read that is not a JSON envelope, so it cannot go through
 * apiFetch — the body is patch text and the truncation flag lives in a response
 * header. apiFetchRaw underneath still supplies the chaos headers, the
 * cold-start transport retry and the ApiError decoding of an error envelope.
 *
 * `maxLines` omitted takes the server's default cap. `maxLines <= 0` is the
 * engine's "unlimited" — the "show all" refetch of a file the cap cut — and is
 * served by the streaming path, which reports no truncation.
 */
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

// ── Search ──────────────────────────────────────────────────────────

/** One match against the CONTENT of the branch diff (gitdomain.SearchHit).
 *
 *  `lineNumber` is a line number in the FILE, not an offset into the patch, and
 *  `side` says which side it counts on: a removed line is numbered on the old
 *  side, an added one on the new side, and a context line — which exists on both
 *  — is reported on the new side, since that is the file the reader navigates
 *  to. `preview` is the line without its diff prefix, capped server-side, so it
 *  may stop before the match itself in a minified file. */
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

/** GET find-in-diff results from the daemon.
 *
 *  An empty query answers an empty result rather than scanning the whole diff,
 *  so it is safe to fire on every keystroke of a find box. An invalid regex is a
 *  400 and surfaces as an ApiError for the caller to render inline. */
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
