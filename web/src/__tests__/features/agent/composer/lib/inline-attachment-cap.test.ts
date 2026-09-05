import { describe, expect, it } from 'vitest'
import {
  exceedsInlineSizeCap,
  INLINE_ATTACHMENT_MAX_BYTES,
} from '@/features/agent/composer/lib/inline-attachment-cap'

describe('exceedsInlineSizeCap', () => {
  it('is false for text well under the cap', () => {
    expect(exceedsInlineSizeCap('hello')).toBe(false)
  })

  it('is false for text exactly at the cap', () => {
    expect(exceedsInlineSizeCap('x'.repeat(INLINE_ATTACHMENT_MAX_BYTES))).toBe(false)
  })

  it('is true for text one byte over the cap', () => {
    expect(exceedsInlineSizeCap('x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1))).toBe(true)
  })

  // A JS string's `.length` counts UTF-16 code units, not bytes — a
  // multi-byte-per-character string near the ASCII-measured cap must still
  // be judged by its real UTF-8 byte length, not undercounted.
  it('measures UTF-8 bytes, not UTF-16 code units', () => {
    // '€' encodes to 3 bytes in UTF-8 but is one UTF-16 code unit.
    const charCount = Math.floor(INLINE_ATTACHMENT_MAX_BYTES / 3) + 1
    const text = '€'.repeat(charCount)
    expect(text.length).toBeLessThanOrEqual(INLINE_ATTACHMENT_MAX_BYTES)
    expect(exceedsInlineSizeCap(text)).toBe(true)
  })
})
