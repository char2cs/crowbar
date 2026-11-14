import { describe, expect, it } from 'vitest'
import { sliceThreadPatch } from '@/features/git/lib/review-thread-snippet'

const HEADER = ['diff --git a/a.ts b/a.ts', '--- a/a.ts', '+++ b/a.ts']
const PATCH = [
  ...HEADER,
  '@@ -1,6 +1,7 @@',
  ' one',
  ' two',
  ' three',
  '-four',
  '+FOUR',
  '+four-b',
  ' five',
  ' six',
  '',
].join('\n')

const anchor = (side: 'old' | 'new', lineNumber: number, startLine = lineNumber) => ({
  filePath: 'a.ts',
  side,
  lineNumber,
  startLine,
})

describe('sliceThreadPatch', () => {
  it('keeps three lines above the anchor, stops at it, and renumbers the hunk', () => {
    const out = sliceThreadPatch(PATCH, anchor('new', 4))
    expect(out?.split('\n').slice(3)).toEqual([
      '@@ -2,3 +2,3 @@',
      ' two',
      ' three',
      '-four',
      '+FOUR',
      '',
    ])
  })

  it('addresses a removed line on the old side', () => {
    const out = sliceThreadPatch(PATCH, anchor('old', 4))
    expect(out).toContain('@@ -1,4 +1,3 @@')
    expect(out).toContain('-four')
    expect(out).not.toContain('+FOUR')
  })

  it('starts a multi-line range at its first line', () => {
    const out = sliceThreadPatch(PATCH, anchor('new', 5, 4))
    expect(out).toContain('+four-b')
    expect(out).toContain('-four')
  })

  it('has nothing to show when the anchor is not in the patch', () => {
    expect(sliceThreadPatch(PATCH, anchor('new', 99))).toBeNull()
    expect(sliceThreadPatch('', anchor('new', 1))).toBeNull()
  })

  it('finds the anchor in a later hunk', () => {
    const patch = [...HEADER, '@@ -1,1 +1,1 @@', ' a', '@@ -50,2 +60,2 @@', ' x', '+y', ''].join(
      '\n',
    )
    const out = sliceThreadPatch(patch, anchor('new', 61))
    expect(out).toContain('@@ -50,1 +60,2 @@')
  })
})
