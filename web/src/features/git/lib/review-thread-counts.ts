import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

/** Unresolved threads per file path; a general comment belongs to no file. */
export function openThreadCountsByPath(threads: readonly ReviewThread[]): Map<string, number> {
  const byPath = new Map<string, number>()
  for (const t of threads) {
    if (!t.isResolved && t.filePath) byPath.set(t.filePath, (byPath.get(t.filePath) ?? 0) + 1)
  }
  return byPath
}
