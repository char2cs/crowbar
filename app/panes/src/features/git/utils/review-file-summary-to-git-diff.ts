import type { ReviewFileSummary } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'

const EMPTY: GitDiff[] = []

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
    is_new: file.status === 'added' || file.status === 'untracked',
    is_deleted: file.status === 'deleted',
    is_renamed: file.status === 'renamed',
    additions: file.additions,
    deletions: file.deletions,
    lines: [],
  }
}
