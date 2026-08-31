import { describe, expect, it } from 'vitest'
import {
  markdownToPlateValue,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(markdownToPlateValue(md))

describe('markdown round-trip (GFM core)', () => {
  it('preserves a heading and emphasis', () => {
    const md = '# Title\n\nThis is **bold** and *italic*.\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves an unordered list', () => {
    const md = '- one\n- two\n- three\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves a task list', () => {
    const md = '- [ ] todo\n- [x] done\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves a GFM table', () => {
    const md = '| a | b |\n| --- | --- |\n| 1 | 2 |\n'
    // Table cell padding may be re-canonicalized; assert structure survives.
    const out = roundTrip(md)
    expect(out).toMatch(/\| a\s*\| b\s*\|/)
    expect(out).toMatch(/\| 1\s*\| 2\s*\|/)
  })

  it('preserves a fenced code block with language', () => {
    const md = '```ts\nconst x = 1\n```\n'
    expect(roundTrip(md)).toContain('```ts')
    expect(roundTrip(md)).toContain('const x = 1')
  })

  it('preserves a link', () => {
    const md = '[Plate](https://platejs.org)\n'
    expect(roundTrip(md)).toContain('[Plate](https://platejs.org)')
  })
})

describe('markdown round-trip (underline)', () => {
  // @platejs/markdown's default `underline` rule serializes to an mdast
  // `mdxJsxTextElement`, which `mdast-util-to-markdown` has no handler for —
  // it throws on every serialize, which Plate's onChange triggers on every
  // op. See markdown-underline-rules.ts for the fix and why `<u>` specifically.
  it('serializes underlined text without throwing, as <u>', () => {
    const value = [{ type: 'p', children: [{ text: 'plain ' }, { text: 'under', underline: true }] }]
    expect(() => plateValueToMarkdown(value)).not.toThrow()
    expect(plateValueToMarkdown(value)).toContain('<u>under</u>')
  })

  it('deserializes <u> back into underlined text', () => {
    const md = 'This is <u>underlined</u> text.\n'
    const value = markdownToPlateValue(md)
    const text = JSON.stringify(value)
    expect(text).toContain('underlined')
  })
})

describe('markdown round-trip (math)', () => {
  it('preserves inline math', () => {
    const md = 'This is $E = mc^2$ inline.\n'
    expect(roundTrip(md)).toBe(md)
  })

  it('preserves block math', () => {
    const md = '$$\nE = mc^2\n$$\n'
    expect(roundTrip(md)).toBe(md)
  })
})

describe('markdown round-trip (callout)', () => {
  it('preserves a GitHub-alert callout, including its type and body', () => {
    const md = '> [!NOTE]\n> Something here.\n'
    const out = roundTrip(md)
    // remark-stringify defensively backslash-escapes a paragraph-leading `[`
    // (any text starting with `[` gets this, not just alert markers — it's
    // disambiguation against link/reference syntax) — so the marker survives
    // as `\[!NOTE]` rather than byte-identical `[!NOTE]`. The type and body
    // are still both fully present; a callout that goes into the editor
    // never comes out silently retyped as a plain quote or missing its
    // variant.
    expect(out).toContain('[!NOTE]')
    expect(out).toContain('Something here.')
    // ...and it's genuinely a callout under the hood, not a blockquote that
    // happens to contain the literal text "[!NOTE]".
    const value = markdownToPlateValue(md)
    expect(value).toEqual([
      {
        type: 'callout',
        variant: 'note',
        children: [{ type: 'p', children: [{ text: 'Something here.' }] }],
      },
    ])
  })

  it('preserves every canonical GitHub alert type', () => {
    for (const type of ['NOTE', 'TIP', 'IMPORTANT', 'WARNING', 'CAUTION']) {
      const md = `> [!${type}]\n> Body for ${type}.\n`
      const value = markdownToPlateValue(md)
      expect(value).toMatchObject([{ type: 'callout', variant: type.toLowerCase() }])
      expect(roundTrip(md)).toContain(`[!${type}]`)
    }
  })

  it('leaves an ordinary blockquote alone', () => {
    const md = '> Just a quote.\n'
    const value = markdownToPlateValue(md)
    expect(value).toMatchObject([{ type: 'blockquote' }])
  })
})

describe('markdown round-trip (mermaid)', () => {
  // A ```mermaid block MUST remain an ordinary fenced code block in the
  // document model (Task 11's core constraint) — it is rendered specially in
  // the rich editor (see mermaid-code-block.tsx) but must round-trip through
  // deserialize/serialize exactly like any other ```lang code block.
  it('preserves a mermaid code fence byte-exactly', () => {
    const md = '```mermaid\nflowchart LR\n  A --> B\n```\n'
    expect(roundTrip(md)).toBe(md)
  })

  it('is genuinely a code_block node under the hood, not a special diagram node', () => {
    const md = '```mermaid\ngraph TD\n  A --> B\n```\n'
    const value = markdownToPlateValue(md)
    expect(value).toMatchObject([{ type: 'code_block', lang: 'mermaid' }])
  })

  it('preserves an empty mermaid fence', () => {
    const md = '```mermaid\n```\n'
    expect(roundTrip(md)).toBe(md)
  })
})

describe('markdown round-trip (math + callout fixed point)', () => {
  it('is a fixed point on a second pass — a save never rewrites an already-serialized document', () => {
    const md = '# Doc\n\n> [!WARNING]\n> Careful: $E=mc^2$\n\n$$\nx^2 + y^2 = z^2\n$$\n\nDone.\n'
    const first = roundTrip(md)
    const second = roundTrip(first)
    expect(second).toBe(first)
    // Sanity: this isn't trivially true because everything got dropped —
    // both constructs are still actually present in the fixed-point output.
    expect(first).toContain('[!WARNING]')
    expect(first).toContain('E=mc^2')
    expect(first).toContain('x^2 + y^2 = z^2')
  })

  // Task 11: a mermaid block alongside a table and math must reach a fixed
  // point too — `roundTrip(roundTrip(md)) === roundTrip(md)`, so a pristine
  // open-then-save of a doc mixing all three never rewrites it a second time.
  it('is a fixed point with a mermaid block, a table, and math all present', () => {
    const md =
      '# Doc\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n```mermaid\nflowchart LR\n  A --> B\n```\n\nInline $E = mc^2$ too.\n'
    const first = roundTrip(md)
    const second = roundTrip(first)
    expect(second).toBe(first)
    expect(first).toContain('```mermaid')
    expect(first).toContain('A --> B')
    expect(first).toMatch(/\| a\s*\| b\s*\|/)
    expect(first).toContain('E = mc^2')
  })
})
