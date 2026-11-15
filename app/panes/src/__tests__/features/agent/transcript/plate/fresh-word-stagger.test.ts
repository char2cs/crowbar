import { describe, expect, it } from 'vitest'
import { splitIntoWords, staggerDelay } from '@/features/agent/transcript/plate/fresh-word-stagger'

describe('splitIntoWords', () => {
  it('concatenates back to the exact original string', () => {
    const cases = [
      'one two three',
      ' leading space',
      'trailing space ',
      'double  space',
      'newline\nseparated\nwords',
      'no-spaces-at-all',
      '',
      '   ',
    ]
    for (const text of cases) {
      expect(splitIntoWords(text).join('')).toBe(text)
    }
  })

  it('splits plain prose into one chunk per word', () => {
    expect(splitIntoWords('the CLI uses argparse')).toEqual(['the ', 'CLI ', 'uses ', 'argparse'])
  })

  it('treats a whitespace-only string as a single chunk', () => {
    expect(splitIntoWords('   ')).toEqual(['   '])
  })

  it('treats an empty string as a single (empty) chunk', () => {
    expect(splitIntoWords('')).toEqual([''])
  })
})

describe('staggerDelay', () => {
  it('is zero for the first word and increases with word index', () => {
    expect(staggerDelay(0, 5)).toBe(0)
    expect(staggerDelay(1, 5)).toBeGreaterThan(staggerDelay(0, 5))
    expect(staggerDelay(2, 5)).toBeGreaterThan(staggerDelay(1, 5))
  })

  it('steps by 30ms for a chunk short enough that never exceeds the cap', () => {
    // 11 words * 30ms = 330ms, just over the 320ms cap — so 10 words is the
    // largest count that still gets the full, uncompressed 30ms step.
    expect(staggerDelay(1, 10)).toBe(30)
    expect(staggerDelay(9, 10)).toBe(270)
  })

  // The per-word STEP scales down rather than the delay being capped, so a long chunk never
  // fades in as one abrupt batch after the first few words.
  it('never gives two different words in the same chunk an identical delay, however long the chunk', () => {
    for (const total of [15, 40, 100]) {
      const delays = Array.from({ length: total }, (_, i) => staggerDelay(i, total))
      expect(new Set(delays).size).toBe(total)
    }
  })

  it('keeps the whole cascade within MAX_STAGGER_MS regardless of chunk length', () => {
    for (const total of [2, 15, 40, 100, 1000]) {
      expect(staggerDelay(total - 1, total)).toBeLessThanOrEqual(320)
    }
  })

  it('is zero for a single-word chunk', () => {
    expect(staggerDelay(0, 1)).toBe(0)
  })
})
