import type { GitDiff } from '../types/git-types'
import type { MultiFileDiff } from '../types/git-diff-types'

/**
 * Guarantees a diff's `lines` is the array its type says it is.
 *
 * `GitDiff.lines` is declared non-nullable and ~a dozen call sites dereference
 * it directly (`diff.lines.length`, `for (const line of diff.lines)`), so a
 * diff arriving without it does not degrade — it takes the whole pane down with
 * "null is not an object".
 *
 * The API no longer sends `null` (see FileDiff.MarshalJSON in the daemon), but
 * this is not merely belt-and-braces: an opened diff tab is PERSISTED with its
 * payload embedded, so a tab opened before that fix reappears with the old
 * shape on the next launch and there is no refetch to correct it. This is the
 * graceful fallback for that, deliberately instead of migration code — the
 * stale value simply reads as empty.
 *
 * Returns the SAME object when nothing needs fixing, so the common path adds no
 * new identity and cannot invalidate a downstream memo.
 */
export function normalizeGitDiff(diff: GitDiff): GitDiff {
  if (Array.isArray(diff.lines)) return diff
  return { ...diff, lines: [] }
}

/** {@link normalizeGitDiff} across a commit/range diff's files. */
export function normalizeMultiFileDiff(multi: MultiFileDiff): MultiFileDiff {
  if (!Array.isArray(multi.files)) return { ...multi, files: [] }

  let repaired = false
  const files = multi.files.map((file) => {
    const next = normalizeGitDiff(file)
    if (next !== file) repaired = true
    return next
  })
  return repaired ? { ...multi, files } : multi
}
