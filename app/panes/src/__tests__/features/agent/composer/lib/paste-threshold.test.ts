import { describe, expect, it } from 'vitest'
import {
  shouldWrapAsTextAttachment,
  PASTE_CHAR_THRESHOLD,
  PASTE_LINE_THRESHOLD,
} from '@/features/agent/composer/lib/paste-threshold'

describe('shouldWrapAsTextAttachment', () => {
  it('stays plain text under both thresholds', () => {
    expect(shouldWrapAsTextAttachment('a short line')).toBe(false)
  })

  it('wraps once the char threshold is exceeded', () => {
    expect(shouldWrapAsTextAttachment('a'.repeat(PASTE_CHAR_THRESHOLD))).toBe(false)
    expect(shouldWrapAsTextAttachment('a'.repeat(PASTE_CHAR_THRESHOLD + 1))).toBe(true)
  })

  it('wraps once the line threshold is exceeded, even if short', () => {
    const text = Array.from({ length: PASTE_LINE_THRESHOLD + 1 }, () => 'x').join('\n')
    expect(shouldWrapAsTextAttachment(text)).toBe(true)
  })

  it('does not wrap at exactly the line threshold', () => {
    const text = Array.from({ length: PASTE_LINE_THRESHOLD }, () => 'x').join('\n')
    expect(shouldWrapAsTextAttachment(text)).toBe(false)
  })
})
