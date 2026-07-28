import { describe, expect, it } from 'vitest'
import { normalizeGitDiff, normalizeMultiFileDiff } from '@/features/git/utils/normalize-diff'
import type { GitDiff } from '@/features/git/types/git-types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

const textFile = (path: string): GitDiff => ({
  file_path: path,
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: [{ line_type: 'added', content: 'x' }] as GitDiff['lines'],
})

// A binary entry as it arrived before the daemon started sending `[]`. Cast
// because the TYPE says this cannot happen — which is exactly why every consumer
// dereferenced it and why the pane crashed instead of degrading.
const binaryFileWithNullLines = (path: string): GitDiff =>
  ({
    file_path: path,
    is_new: true,
    is_deleted: false,
    is_renamed: false,
    is_binary: true,
    lines: null,
  }) as unknown as GitDiff

describe('normalizeGitDiff', () => {
  it('replaces a null `lines` with an empty array', () => {
    const out = normalizeGitDiff(binaryFileWithNullLines('assets/blob.bin'))
    expect(out.lines).toEqual([])
    expect(() => out.lines.length).not.toThrow()
  })

  it('returns the SAME object when nothing needs repair', () => {
    // Identity matters: a fresh object here would invalidate the memos
    // downstream on every render of a diff that was never broken.
    const diff = textFile('src/a.ts')
    expect(normalizeGitDiff(diff)).toBe(diff)
  })

  it('leaves real lines untouched', () => {
    const diff = textFile('src/a.ts')
    expect(normalizeGitDiff(diff).lines).toHaveLength(1)
  })
})

describe('normalizeMultiFileDiff', () => {
  it('repairs only the broken entries and keeps the good ones by identity', () => {
    const good = textFile('src/a.ts')
    const bad = binaryFileWithNullLines('assets/blob.bin')
    const multi = { title: 'Commit abc', files: [good, bad] } as unknown as MultiFileDiff

    const out = normalizeMultiFileDiff(multi)
    expect(out.files[0]).toBe(good)
    expect(out.files[1].lines).toEqual([])
  })

  it('returns the SAME object when every file is already well-formed', () => {
    const multi = { title: 'Commit abc', files: [textFile('src/a.ts')] } as unknown as MultiFileDiff
    expect(normalizeMultiFileDiff(multi)).toBe(multi)
  })

  it('survives a payload with no files array at all', () => {
    const multi = { title: 'Commit abc', files: null } as unknown as MultiFileDiff
    expect(normalizeMultiFileDiff(multi).files).toEqual([])
  })
})
