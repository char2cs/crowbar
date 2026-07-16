import { describe, expect, it } from 'vitest'
import { gitStatusToChangedFiles } from '@/features/git/utils/git-status-to-changed-files'
import type { GitFile } from '@/features/git/types/git-types'

function file(path: string, status: GitFile['status'], staged = false): GitFile {
  return { path, status, staged }
}

describe('gitStatusToChangedFiles', () => {
  it('returns a referentially-stable empty array for null/undefined/empty input', () => {
    const a = gitStatusToChangedFiles(null)
    const b = gitStatusToChangedFiles(undefined)
    const c = gitStatusToChangedFiles([])
    expect(a).toEqual([])
    // Stable-empty rule: same reference across calls so memoized consumers
    // don't re-render when there are no changes.
    expect(a).toBe(b)
    expect(b).toBe(c)
  })

  it('maps every git status kind onto the GitDiff is_* flags', () => {
    const out = gitStatusToChangedFiles([
      file('src/mod.ts', 'modified'),
      file('src/new.ts', 'added'),
      file('src/gone.ts', 'deleted'),
      file('src/moved.ts', 'renamed'),
      file('src/untracked.ts', 'untracked'),
    ])

    expect(out).toEqual([
      { file_path: 'src/mod.ts', is_new: false, is_deleted: false, is_renamed: false, lines: [] },
      { file_path: 'src/new.ts', is_new: true, is_deleted: false, is_renamed: false, lines: [] },
      { file_path: 'src/gone.ts', is_new: false, is_deleted: true, is_renamed: false, lines: [] },
      { file_path: 'src/moved.ts', is_new: false, is_deleted: false, is_renamed: true, lines: [] },
      // untracked surfaces as a new file
      {
        file_path: 'src/untracked.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        lines: [],
      },
    ])
  })

  it('carries no per-file line counts or uncommitted flag (unavailable from status)', () => {
    const [entry] = gitStatusToChangedFiles([file('a.ts', 'modified')])
    expect(entry.additions).toBeUndefined()
    expect(entry.deletions).toBeUndefined()
    expect(entry.uncommitted).toBeUndefined()
  })

  it('dedupes a path listed twice (staged + unstaged) into a single entry', () => {
    const out = gitStatusToChangedFiles([
      file('src/a.ts', 'modified', true),
      file('src/a.ts', 'modified', false),
      file('src/b.ts', 'modified', false),
    ])
    expect(out.map((f) => f.file_path)).toEqual(['src/a.ts', 'src/b.ts'])
  })
})
