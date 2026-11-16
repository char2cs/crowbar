import { describe, expect, it } from 'vitest'
import { splitIntoWords, staggerDelay } from '@/features/agent/transcript/plate/fresh-word-stagger'

describe('splitIntoWords', () => {
  it.each([
    'one two three',
    ' leading space',
    'trailing space ',
    'double  space',
    'newline\nseparated\nwords',
    'no-spaces-at-all',
    '',
    '   ',
  ])('concatenates back to the exact original string %j', (text) => {
    expect(splitIntoWords(text).join('')).toBe(text)
  })

  it.each([
    [
      'plain prose into one chunk per word',
      'the CLI uses argparse',
      ['the ', 'CLI ', 'uses ', 'argparse'],
    ],
    ['a whitespace-only string into a single chunk', '   ', ['   ']],
    ['an empty string into a single empty chunk', '', ['']],
  ])('splits %s', (_name, text, chunks) => {
    expect(splitIntoWords(text)).toEqual(chunks)
  })
})

describe('staggerDelay', () => {
  it.each([
    ['is zero for the first word', 0, 5, 0],
    ['is zero for a single-word chunk', 0, 1, 0],
    ['steps by 30ms in a chunk short enough to stay under the cap', 1, 10, 30],
    ['keeps the full step to the last word of the longest uncompressed chunk', 9, 10, 270],
  ])('%s', (_name, index, total, delay) => {
    expect(staggerDelay(index, total)).toBe(delay)
  })

  it.each([5, 15, 40, 100])(
    'gives every word of a %i-word chunk a later delay than the one before',
    (total) => {
      const delays = Array.from({ length: total }, (_, i) => staggerDelay(i, total))
      expect(delays.every((delay, i) => i === 0 || delay > delays[i - 1])).toBe(true)
    },
  )

  it.each([2, 15, 40, 100, 1000])('ends a %i-word cascade within 320ms', (total) => {
    expect(staggerDelay(total - 1, total)).toBeLessThanOrEqual(320)
  })
})
