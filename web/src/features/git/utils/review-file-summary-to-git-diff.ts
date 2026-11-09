import type { ReviewFileSummary } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'

// Stable-empty rule: reuse one module-level empty array so an empty summary maps
// to a referentially stable result across renders (mirrors git-status-to-changed-files).
const EMPTY: GitDiff[] = []

/** The files summary as GitDiff[], with ± counts. `lines` stays empty — the summary has no
 *  content; binaries' -1 counts are hidden by SidebarFileRowCounts. */
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
    lines: [],
  }
}
