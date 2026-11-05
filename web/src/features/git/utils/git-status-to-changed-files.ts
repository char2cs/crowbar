import type { GitDiff, GitFile } from '../types/git-types'

// One module-level empty array keeps an empty projection referentially stable across renders.
const EMPTY: GitDiff[] = []

/** The working-tree status as the sidebar tree's GitDiff[], shown until the files summary loads.
 *  Status has no line counts, and every row is uncommitted, so neither is set. A path listed
 *  staged and unstaged appears once (first wins), as in the branch diff. */
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
