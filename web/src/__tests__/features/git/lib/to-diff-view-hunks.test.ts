import { describe, it, expect } from 'vitest'
import { gitDiffToHunks } from '@/features/git/lib/to-diff-view-hunks'
import type { GitDiff } from '@/features/git/types/git-types'

describe('gitDiffToHunks', () => {
  it('maps a GitDiff with 1 header + removed + added + context to one HunkData with three changes', () => {
    const diff: GitDiff = {
      file_path: 'src/foo.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      lines: [
        {
          line_type: 'header',
          content: '@@ -10,3 +10,3 @@',
          old_line_number: undefined,
          new_line_number: undefined,
        },
        {
          line_type: 'removed',
          content: 'const x = 1',
          old_line_number: 10,
          new_line_number: undefined,
        },
        {
          line_type: 'added',
          content: 'const x = 2',
          old_line_number: undefined,
          new_line_number: 10,
        },
        {
          line_type: 'context',
          content: 'export { x }',
          old_line_number: 11,
          new_line_number: 11,
        },
      ],
    }

    const hunks = gitDiffToHunks(diff)

    expect(hunks).toHaveLength(1)
    const hunk = hunks[0]!

    // Header content preserved from GitDiffLine
    expect(hunk.content).toBe('@@ -10,3 +10,3 @@')

    // Line starts computed from the first old/new line numbers
    expect(hunk.oldStart).toBe(10)
    expect(hunk.newStart).toBe(10)

    // oldLines = delete(1) + normal(1) = 2; newLines = insert(1) + normal(1) = 2
    expect(hunk.oldLines).toBe(2)
    expect(hunk.newLines).toBe(2)

    expect(hunk.changes).toHaveLength(3)

    // First: delete change
    const del = hunk.changes[0]!
    expect(del.type).toBe('delete')
    expect(del.content).toBe('const x = 1')
    if (del.type !== 'delete') throw new Error('expected delete')
    expect(del.isDelete).toBe(true)
    expect(del.lineNumber).toBe(10)

    // Second: insert change
    const ins = hunk.changes[1]!
    expect(ins.type).toBe('insert')
    expect(ins.content).toBe('const x = 2')
    if (ins.type !== 'insert') throw new Error('expected insert')
    expect(ins.isInsert).toBe(true)
    expect(ins.lineNumber).toBe(10)

    // Third: normal (context) change
    const ctx = hunk.changes[2]!
    expect(ctx.type).toBe('normal')
    expect(ctx.content).toBe('export { x }')
    if (ctx.type !== 'normal') throw new Error('expected normal')
    expect(ctx.isNormal).toBe(true)
    expect(ctx.oldLineNumber).toBe(11)
    expect(ctx.newLineNumber).toBe(11)
  })

  it('synthesizes a single hunk from line 1 for a pure-new file (all added, no header)', () => {
    const diff: GitDiff = {
      file_path: 'src/new-file.ts',
      is_new: true,
      is_deleted: false,
      is_renamed: false,
      lines: [
        { line_type: 'added', content: 'line one', old_line_number: undefined, new_line_number: 1 },
        { line_type: 'added', content: 'line two', old_line_number: undefined, new_line_number: 2 },
        {
          line_type: 'added',
          content: 'line three',
          old_line_number: undefined,
          new_line_number: 3,
        },
      ],
    }

    const hunks = gitDiffToHunks(diff)

    expect(hunks).toHaveLength(1)
    const hunk = hunks[0]!

    // Synthesized header
    expect(hunk.content).toBe('@@ -1,0 +1,3 @@')

    expect(hunk.oldStart).toBe(1)
    expect(hunk.newStart).toBe(1)
    expect(hunk.oldLines).toBe(0)
    expect(hunk.newLines).toBe(3)

    expect(hunk.changes).toHaveLength(3)

    for (let i = 0; i < 3; i++) {
      const change = hunk.changes[i]!
      expect(change.type).toBe('insert')
      if (change.type !== 'insert') throw new Error('expected insert')
      expect(change.isInsert).toBe(true)
      expect(change.lineNumber).toBe(i + 1)
    }
  })

  it('handles multiple hunks (two header blocks)', () => {
    const diff: GitDiff = {
      file_path: 'src/bar.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      lines: [
        { line_type: 'header', content: '@@ -1,1 +1,1 @@', old_line_number: undefined, new_line_number: undefined },
        { line_type: 'removed', content: 'old line 1', old_line_number: 1, new_line_number: undefined },
        { line_type: 'added', content: 'new line 1', old_line_number: undefined, new_line_number: 1 },
        { line_type: 'header', content: '@@ -50,1 +50,1 @@', old_line_number: undefined, new_line_number: undefined },
        { line_type: 'removed', content: 'old line 50', old_line_number: 50, new_line_number: undefined },
        { line_type: 'added', content: 'new line 50', old_line_number: undefined, new_line_number: 50 },
      ],
    }

    const hunks = gitDiffToHunks(diff)

    expect(hunks).toHaveLength(2)
    expect(hunks[0]!.oldStart).toBe(1)
    expect(hunks[1]!.oldStart).toBe(50)
    expect(hunks[0]!.changes).toHaveLength(2)
    expect(hunks[1]!.changes).toHaveLength(2)
  })

  it('returns empty array for a diff with no lines', () => {
    const diff: GitDiff = {
      file_path: 'src/empty.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      lines: [],
    }

    const hunks = gitDiffToHunks(diff)
    expect(hunks).toHaveLength(0)
  })
})
