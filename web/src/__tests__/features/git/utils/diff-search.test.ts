import { describe, it, expect } from 'vitest'
import { computeDiffMatches, MAX_DIFF_MATCHES } from '@/features/git/utils/diff-search'

const keyFor = (i: number) => `file-${i}`
const opts = { caseSensitive: false, wholeWord: false, useRegex: false }

describe('computeDiffMatches', () => {
  it('finds matches across files with file + line + column positions', () => {
    const contents = [
      'const Container = 1\nfoo()', // file 0: "Container" at line 1, col 7
      'type Container struct {\n  inner Container\n}', // file 1: two matches
    ]
    const { matches } = computeDiffMatches(contents, keyFor, 'Container', opts)

    expect(matches).toHaveLength(3)
    expect(matches[0]).toMatchObject({
      fileKey: 'file-0',
      fileIndex: 0,
      lineNumber: 1,
      startColumn: 7,
      endColumn: 16,
    })
    expect(matches[1]).toMatchObject({ fileKey: 'file-1', fileIndex: 1, lineNumber: 1 })
    expect(matches[2]).toMatchObject({ fileKey: 'file-1', fileIndex: 1, lineNumber: 2 })
  })

  it('finds multiple matches on the same line', () => {
    const { matches } = computeDiffMatches(['foo foo foo'], keyFor, 'foo', opts)
    expect(matches).toHaveLength(3)
    expect(matches.map((m) => m.startColumn)).toEqual([1, 5, 9])
  })

  it('is case-insensitive by default and case-sensitive when set', () => {
    const contents = ['Foo foo FOO']
    expect(computeDiffMatches(contents, keyFor, 'foo', opts).matches).toHaveLength(3)
    expect(
      computeDiffMatches(contents, keyFor, 'foo', { ...opts, caseSensitive: true }).matches,
    ).toHaveLength(1)
  })

  it('returns no matches for an empty query', () => {
    expect(computeDiffMatches(['abc'], keyFor, '', opts).matches).toHaveLength(0)
  })

  it('returns no matches when nothing matches', () => {
    const { matches, limited } = computeDiffMatches(['hello world'], keyFor, 'zzz', opts)
    expect(matches).toHaveLength(0)
    expect(limited).toBe(false)
  })

  it('caps at MAX_DIFF_MATCHES and flags limited', () => {
    const huge = Array.from({ length: MAX_DIFF_MATCHES + 50 }, () => 'x').join('\n')
    const { matches, limited } = computeDiffMatches([huge], keyFor, 'x', opts)
    expect(matches.length).toBe(MAX_DIFF_MATCHES)
    expect(limited).toBe(true)
  })

  it('treats the query literally (regex metacharacters are escaped)', () => {
    const { matches } = computeDiffMatches(['a.b a_b axb'], keyFor, 'a.b', opts)
    // Only the literal "a.b" matches, not the regex "a<any>b".
    expect(matches).toHaveLength(1)
    expect(matches[0].startColumn).toBe(1)
  })
})
