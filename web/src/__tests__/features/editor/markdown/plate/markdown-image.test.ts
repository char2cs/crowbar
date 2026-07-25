import { describe, expect, it } from 'vitest'
import {
  markdownToPlateValue,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(markdownToPlateValue(md))

describe('markdown image round-trip', () => {
  it('preserves a simple local image', () => {
    const md = '![Logo](public/logo.png)\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves alt text and a title', () => {
    const md = '![Alt text](img/x.png "A title")\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves a remote image', () => {
    const md = '![](https://example.com/badge.svg)\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('keeps content when an image sits inline among text (reflowed to its own block, not dropped)', () => {
    // @platejs lifts an image out of a paragraph into its own block, so an
    // inline image reflows to its own line. That's a re-canonicalization, not
    // data loss — the surrounding text and the image all survive.
    const out = roundTrip('Before ![i](a.png) after.\n')
    expect(out).toContain('Before')
    expect(out).toContain('![i](a.png)')
    expect(out).toContain('after.')
  })

  it('is a fixed point', () => {
    const md = '![Logo](./assets/logo.png "Logo")\n\ntext\n'
    const once = roundTrip(md)
    expect(roundTrip(once)).toBe(once)
  })
})
