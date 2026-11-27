import { describe, expect, it } from 'vitest'
import {
  parseMarkdown,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(parseMarkdown(md).value)

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
