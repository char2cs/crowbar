import { describe, expect, test } from 'vitest'
import type { GitDiff, GitDiffLine } from '@/features/git/types/git-types'
import {
  buildMonacoDiffContent,
  buildUnifiedThreadAnchorMap,
  findUnifiedModelLine,
  serializeGitDiffSourceForEditor,
} from '@/features/git/utils/diff-editor-content'

type RawLine = {
  type: GitDiffLine['line_type']
  content: string
  old?: number
  new?: number
}
const makeRawDiff = (lines: RawLine[]): GitDiff => ({
  file_path: 'src/foo.ts',
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: lines.map((l) => ({
    line_type: l.type,
    content: l.content,
    old_line_number: l.old,
    new_line_number: l.new,
  })),
})

const makeDiff = (lines: Array<{ type: GitDiffLine['line_type']; content: string }>): GitDiff => ({
  file_path: 'src/foo.ts',
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: lines.map((l, i) => ({
    line_type: l.type,
    content: l.content,
    old_line_number: i + 1,
    new_line_number: i + 1,
  })),
})

describe('buildMonacoDiffContent', () => {
  test('context lines appear on both sides', () => {
    const diff = makeDiff([{ type: 'context', content: 'const x = 1' }])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('const x = 1')
    expect(modified).toBe('const x = 1')
  })

  test('removed lines appear in original only', () => {
    const diff = makeDiff([{ type: 'removed', content: 'old line' }])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('old line')
    expect(modified).toBe('')
  })

  test('added lines appear in modified only', () => {
    const diff = makeDiff([{ type: 'added', content: 'new line' }])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('')
    expect(modified).toBe('new line')
  })

  test('header lines are excluded from both sides', () => {
    const diff = makeDiff([
      { type: 'header', content: '@@ -1,3 +1,3 @@' },
      { type: 'context', content: 'kept' },
    ])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('kept')
    expect(modified).toBe('kept')
  })

  test('mixed diff produces correct original and modified strings', () => {
    const diff = makeDiff([
      { type: 'context', content: 'line1' },
      { type: 'removed', content: 'old' },
      { type: 'added', content: 'new' },
      { type: 'context', content: 'line4' },
    ])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('line1\nold\nline4')
    expect(modified).toBe('line1\nnew\nline4')
  })

  test('empty diff produces empty strings', () => {
    const diff = makeDiff([])
    const { original, modified } = buildMonacoDiffContent(diff)
    expect(original).toBe('')
    expect(modified).toBe('')
  })

  test('raw_patch diff with empty lines produces empty strings', () => {
    const diff: GitDiff = {
      file_path: 'src/big.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      raw_patch: '--- a/src/big.ts\n+++ b/src/big.ts\n@@ -1 +1 @@\n-old\n+new',
      lines: [],
    }
    const { original, modified } = buildMonacoDiffContent(diff)
    // raw_patch is not parsed by buildMonacoDiffContent; callers must handle this case
    expect(original).toBe('')
    expect(modified).toBe('')
  })
})

describe('unified thread anchor map', () => {
  // A diff where old/new numbers diverge after the inserted line, so the
  // side-aware map must be consulted (the collapsed actualLines would be wrong
  // for the old side of context lines below the insertion).
  const diff = makeRawDiff([
    { type: 'header', content: '@@ -5,4 +5,5 @@' },
    { type: 'context', content: 'a', old: 5, new: 5 },
    { type: 'removed', content: 'b', old: 6 },
    { type: 'added', content: 'c', new: 6 },
    { type: 'added', content: 'd', new: 7 },
    { type: 'context', content: 'e', old: 7, new: 8 },
  ])

  test('anchor map aligns 1:1 with the unified reconstruction (model line i → entry i)', () => {
    const anchors = buildUnifiedThreadAnchorMap(diff)
    const { content } = serializeGitDiffSourceForEditor(diff)
    // One anchor entry per reconstructed model line.
    expect(anchors).toHaveLength(content.split('\n').length)
    expect(anchors).toEqual([
      { oldLine: 5, newLine: 5 }, // context a
      { oldLine: 6, newLine: null }, // removed b
      { oldLine: null, newLine: 6 }, // added c
      { oldLine: null, newLine: 7 }, // added d
      { oldLine: 7, newLine: 8 }, // context e (old/new diverge)
    ])
  })

  test('inverts {side,line} to the correct 1-based model line', () => {
    const anchors = buildUnifiedThreadAnchorMap(diff)
    expect(findUnifiedModelLine(anchors, 'new', 5)).toBe(1)
    expect(findUnifiedModelLine(anchors, 'old', 6)).toBe(2) // removed line, old side
    expect(findUnifiedModelLine(anchors, 'new', 6)).toBe(3) // added line, new side
    expect(findUnifiedModelLine(anchors, 'new', 7)).toBe(4)
    // Context line whose old (7) and new (8) differ — both resolve to its row.
    expect(findUnifiedModelLine(anchors, 'old', 7)).toBe(5)
    expect(findUnifiedModelLine(anchors, 'new', 8)).toBe(5)
  })

  test('returns null when the anchored line is no longer in the diff (outdated)', () => {
    const anchors = buildUnifiedThreadAnchorMap(diff)
    expect(findUnifiedModelLine(anchors, 'new', 99)).toBeNull()
    expect(findUnifiedModelLine(anchors, 'old', 6)).not.toBeNull()
    expect(findUnifiedModelLine(anchors, 'new', 6)).not.toBeNull()
    // 'old' side never had line 6's new number, etc.
    expect(findUnifiedModelLine(anchors, 'old', 8)).toBeNull()
  })
})
