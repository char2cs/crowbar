import { describe, expect, it } from 'vitest'
import { fitTitle } from '@/sdk/fit-title'

const LIMIT = 256

describe('fitTitle', () => {
  it.each([
    ['trims whitespace', '  Fix the build  ', 'Fix the build'],
    ['turns control characters into spaces', 'a\nb', 'a b'],
    ['keeps a title within the limit', 'x'.repeat(LIMIT), 'x'.repeat(LIMIT)],
  ])('%s', (_name, title, expected) => {
    expect(fitTitle(title)).toBe(expected)
  })

  it('cuts a long multi-byte title within the byte limit and marks the cut', () => {
    const fitted = fitTitle('é'.repeat(LIMIT))
    expect(new TextEncoder().encode(fitted).length).toBeLessThanOrEqual(LIMIT)
    expect(fitted.endsWith('...')).toBe(true)
  })
})
