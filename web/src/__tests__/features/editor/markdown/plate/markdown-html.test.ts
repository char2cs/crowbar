import { describe, expect, it } from 'vitest'
import {
  markdownToPlateValue,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(markdownToPlateValue(md))

describe('raw HTML block preservation', () => {
  it('round-trips a centered README header block byte-exact', () => {
    const md = `<div align="center">
  <img src="public/logo.png" alt="Athas" width="120" />
  <h1>Athas</h1>
  <p>A lightweight editor with <a href="https://tauri.app/">Tauri</a>.</p>
</div>
`
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('does not escape angle brackets or spaces', () => {
    const out = roundTrip('<div class="x">\n  <span>hi</span>\n</div>\n')
    expect(out).not.toContain('\\<')
    expect(out).not.toContain('&#x20;')
    expect(out).toContain('<div class="x">')
    expect(out).toContain('<span>hi</span>')
  })

  it('preserves a self-closing HTML comment / details block', () => {
    const md = `<details>
<summary>Click</summary>

Hidden content.

</details>
`
    // details wrapping markdown: the HTML tags survive; inner markdown may
    // re-canonicalize but the structural tags must not be escaped.
    const out = roundTrip(md)
    expect(out).toContain('<details>')
    expect(out).toContain('<summary>Click</summary>')
    expect(out).toContain('</details>')
    expect(out).not.toContain('\\<')
  })

  it('leaves normal markdown untouched (no false HTML capture)', () => {
    const md = '# Heading\n\nA paragraph with **bold**.\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('is a fixed point: a second pass changes nothing', () => {
    const md = `<div align="center"><img src="a.png" /></div>\n\n# After\n`
    const once = roundTrip(md)
    expect(roundTrip(once)).toBe(once)
  })
})
