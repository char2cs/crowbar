import type { ReviewFileSummary } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'

// Stable-empty rule: reuse one module-level empty array so an empty summary maps
// to a referentially stable result across renders (mirrors git-status-to-changed-files).
const EMPTY: GitDiff[] = []

/**
 * Map the files-only branch-review summary (`ReviewFileSummary[]`) into the
 * `GitDiff[]` shape the sidebar `ChangedFilesTree` consumes. Unlike the raw
 * status projection, the summary carries per-file `additions`/`deletions`, so
 * the +N/-N badges render, and its `uncommitted` flag is honest — committed-only
 * files map to `uncommitted: false` (no "uncommitted" badge) while working-tree
 * changes stay flagged.
 *
 * `lines` is left empty on purpose: the summary has no line content (that is the
 * whole point — the sidebar never pays for the line-level diff). Binary files
 * carry -1 counts, which `GitFileItem` already hides (it only renders counts > 0).
 */
export function reviewFilesSummaryToChangedFiles(
  files: ReviewFileSummary[] | undefined | null,
): GitDiff[] {
  if (!files || files.length === 0) return EMPTY
  return files.map(reviewFileSummaryToGitDiff)
}

function reviewFileSummaryToGitDiff(file: ReviewFileSummary): GitDiff {
  return {
    file_path: file.path,
    old_path: file.old_path,
    // 'untracked' has no committed counterpart — render it as an addition,
    // matching how the full branch diff surfaces brand-new files (is_new).
    is_new: file.status === 'added' || file.status === 'untracked',
    is_deleted: file.status === 'deleted',
    is_renamed: file.status === 'renamed',
    additions: file.additions,
    deletions: file.deletions,
    uncommitted: file.uncommitted,
    lines: [],
  }
}
