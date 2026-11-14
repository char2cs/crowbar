import { describe, expect, it } from 'vitest'
import {
  commentMarkdownToValue,
  commentValueToMarkdown,
} from '@/features/editor/markdown/plate/comment/comment-serialization'

const roundTrip = (md: string) => commentValueToMarkdown(commentMarkdownToValue(md))

// The comment editor runs a REDUCED plugin set (see comment-plugins.tsx), and
// @platejs/markdown drops any node whose plugin is absent. That makes this file
// the gate on the trim: every construct here is one a reviewer plausibly writes,
// and a construct that does not survive is not a missing affordance — it is a
// comment that loses content the moment its author edits it to fix a typo.
//
// Assertions are on CONTENT rather than bytes. The round trip re-canonicalizes
// punctuation and cell padding (the file editor has the same property, which is
// why it keeps a baseline instead of comparing to the original), so byte
// equality would fail for reasons that cost the user nothing.
describe('comment markdown round-trip: what a review comment contains', () => {
  it('preserves prose with marks', () => {
    const md = 'This is **bold**, *italic*, ~~struck~~ and `inline code`.'
    expect(roundTrip(md).trim()).toBe(md)
  })

  it('preserves headings', () => {
    expect(roundTrip('## Suggestion\n\nDo the thing.\n').trim()).toBe(
      '## Suggestion\n\nDo the thing.',
    )
  })

  it('preserves bulleted, numbered and task lists', () => {
    expect(roundTrip('- one\n- two\n').trim()).toBe('- one\n- two')
    expect(roundTrip('1. one\n2. two\n').trim()).toBe('1. one\n2. two')
    expect(roundTrip('- [ ] todo\n- [x] done\n').trim()).toBe('- [ ] todo\n- [x] done')
  })

  it('preserves a blockquote', () => {
    expect(roundTrip('> quoting the diff\n').trim()).toBe('> quoting the diff')
  })

  it('preserves a fenced code block with its language', () => {
    const out = roundTrip('```ts\nconst x = 1\n```\n')
    expect(out).toContain('```ts')
    expect(out).toContain('const x = 1')
  })

  it('preserves a link', () => {
    expect(roundTrip('see [the docs](https://example.com/a)')).toContain(
      '[the docs](https://example.com/a)',
    )
  })

  it('preserves an image', () => {
    expect(roundTrip('![screenshot](https://example.com/s.png)')).toContain(
      '![screenshot](https://example.com/s.png)',
    )
  })

  it('preserves a GFM table', () => {
    const out = roundTrip('| case | ms |\n| --- | --- |\n| before | 40 |\n')
    expect(out).toMatch(/\| case\s*\| ms\s*\|/)
    expect(out).toMatch(/\| before\s*\| 40\s*\|/)
  })

  it('preserves a GitHub alert', () => {
    const out = roundTrip('> [!WARNING]\n> This allocates per row.\n')
    expect(out).toContain('[!WARNING]')
    expect(out).toContain('This allocates per row.')
  })

  it('preserves a raw HTML block', () => {
    // `<details>` is how a reviewer folds a long log into a comment — and it is
    // markup no node type here models, so it rides through HtmlKit verbatim.
    const md = '<details>\n<summary>full output</summary>\n\nplenty of text\n\n</details>'
    expect(roundTrip(md)).toContain('<details>')
    expect(roundTrip(md)).toContain('<summary>full output</summary>')
  })

  it('leaves dollar signs alone', () => {
    // No math plugin and no remark-math, so `$` is the literal character a
    // reviewer typed — most often inside a shell snippet, not an equation.
    expect(roundTrip('run `$HOME/bin/x` and pay $5')).toContain('$HOME/bin/x')
  })

  it('survives a comment that mixes everything', () => {
    const md = [
      'Two things:',
      '',
      '1. `parseRow` allocates per row — see [the profile](https://example.com/p).',
      '2. The **fast path** should be:',
      '',
      '```go',
      'if n == 0 { return nil }',
      '```',
      '',
      '> [!NOTE]',
      '> Numbers from the 1M-line fixture.',
    ].join('\n')

    const out = roundTrip(md)
    expect(out).toContain('`parseRow`')
    expect(out).toContain('[the profile](https://example.com/p)')
    expect(out).toContain('**fast path**')
    expect(out).toContain('```go')
    expect(out).toContain('if n == 0 { return nil }')
    expect(out).toContain('[!NOTE]')
  })
})

describe('comment markdown round-trip: empty input', () => {
  it('turns an empty comment into an empty document, not a stray character', () => {
    // Plate's init synthesizes a paragraph for an empty document. If that
    // synthesized node serialized to anything, a composer the user opened and
    // closed would post — or enable its Comment button on — whitespace.
    expect(commentValueToMarkdown(commentMarkdownToValue('')).trim()).toBe('')
  })
})
