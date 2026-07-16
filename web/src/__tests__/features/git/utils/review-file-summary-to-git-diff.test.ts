import { describe, expect, it } from 'vitest'
import type { ReviewFileSummary } from '@/features/git/api/review-api'
import { reviewFilesSummaryToChangedFiles } from '@/features/git/utils/review-file-summary-to-git-diff'

function summaryFile(
  path: string,
  status: ReviewFileSummary['status'],
  extra: Partial<ReviewFileSummary> = {},
): ReviewFileSummary {
  return { path, status, additions: 0, deletions: 0, uncommitted: false, staged: false, ...extra }
}

describe('reviewFilesSummaryToChangedFiles', () => {
  it('returns a stable empty array for empty/nullish input', () => {
    const a = reviewFilesSummaryToChangedFiles([])
    const b = reviewFilesSummaryToChangedFiles(undefined)
    const c = reviewFilesSummaryToChangedFiles(null)
    expect(a).toEqual([])
    // Same reference each call (stable-empty rule) so consumers don't churn.
    expect(a).toBe(b)
    expect(b).toBe(c)
  })

  it('maps added and untracked to is_new', () => {
    const [added, untracked] = reviewFilesSummaryToChangedFiles([
      summaryFile('a.ts', 'added', { additions: 5 }),
      summaryFile('b.ts', 'untracked', { uncommitted: true }),
    ])
    expect(added.is_new).toBe(true)
    expect(added.additions).toBe(5)
    expect(untracked.is_new).toBe(true)
    expect(untracked.uncommitted).toBe(true)
  })

  it('maps deleted and modified with their counts and uncommitted flag', () => {
    const [deleted, modified] = reviewFilesSummaryToChangedFiles([
      summaryFile('gone.ts', 'deleted', { deletions: 9 }),
      summaryFile('keep.ts', 'modified', { additions: 3, deletions: 1, uncommitted: true }),
    ])
    expect(deleted.is_deleted).toBe(true)
    expect(deleted.deletions).toBe(9)
    expect(modified.is_new).toBe(false)
    expect(modified.is_deleted).toBe(false)
    expect(modified.is_renamed).toBe(false)
    expect(modified.additions).toBe(3)
    expect(modified.uncommitted).toBe(true)
  })

  it('maps renamed carrying the old path', () => {
    const [renamed] = reviewFilesSummaryToChangedFiles([
      summaryFile('new.ts', 'renamed', { old_path: 'old.ts' }),
    ])
    expect(renamed.is_renamed).toBe(true)
    expect(renamed.old_path).toBe('old.ts')
    expect(renamed.file_path).toBe('new.ts')
  })

  it('preserves the -1 binary counts (GitFileItem hides counts <= 0)', () => {
    const [bin] = reviewFilesSummaryToChangedFiles([
      summaryFile('logo.png', 'modified', { additions: -1, deletions: -1 }),
    ])
    expect(bin.additions).toBe(-1)
    expect(bin.deletions).toBe(-1)
  })

  it('committed-only files carry uncommitted=false (no badge)', () => {
    const [committed] = reviewFilesSummaryToChangedFiles([
      summaryFile('src/x.ts', 'modified', { additions: 4, uncommitted: false }),
    ])
    expect(committed.uncommitted).toBe(false)
    expect(committed.additions).toBe(4)
  })
})
