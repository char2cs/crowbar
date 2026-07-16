import type { GitDiff, GitFile } from '../types/git-types'

// Stable-empty rule: reuse one module-level empty array so an empty status
// projection is referentially stable across renders (see use-review-diff.ts).
const EMPTY: GitDiff[] = []

/**
 * Project the cheap working-tree git status (streamed into the git store) into
 * the `GitDiff[]` shape the sidebar `ChangedFilesTree` consumes, WITHOUT the
 * full line-level branch diff.
 *
 * This drives the always-mounted sidebar changed-files list while the Branch
 * Review pane is closed, so the expensive `getReview` fetch only runs while the
 * review pane is actually open (P2b).
 *
 * The working-tree status payload (`GitFile`: path/status/staged) carries no
 * per-file line counts, so `additions`/`deletions` are intentionally omitted
 * (the +N/-N badges only appear with the full diff, i.e. while review is open).
 * `uncommitted` is likewise omitted: every file here IS an uncommitted
 * working-tree change, so a per-row "uncommitted" badge would be noise.
 *
 * `git status` can list the same path twice (a staged entry AND an unstaged
 * entry); the branch diff shows one row per file, so we dedupe by path
 * (first occurrence wins) to keep the tree visually identical.
 */
export function gitStatusToChangedFiles(files: GitFile[] | undefined | null): GitDiff[] {
  if (!files || files.length === 0) return EMPTY

  const seen = new Set<string>()
  const out: GitDiff[] = []
  for (const file of files) {
    if (seen.has(file.path)) continue
    seen.add(file.path)
    out.push({
      file_path: file.path,
      // 'untracked' has no committed counterpart — render it as an addition,
      // matching how the full branch diff surfaces brand-new files (is_new).
      is_new: file.status === 'added' || file.status === 'untracked',
      is_deleted: file.status === 'deleted',
      is_renamed: file.status === 'renamed',
      lines: [],
    })
  }
  return out
}
