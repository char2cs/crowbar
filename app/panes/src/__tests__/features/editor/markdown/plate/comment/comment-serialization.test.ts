import { describe, expect, it } from 'vitest'
import {
  commentMarkdownToValue,
  commentValueToMarkdown,
} from '@/features/editor/markdown/plate/comment/comment-serialization'

const roundTrip = (md: string) => commentValueToMarkdown(commentMarkdownToValue(md))

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
    const md = '<details>\n<summary>full output</summary>\n\nplenty of text\n\n</details>'
    expect(roundTrip(md)).toContain('<details>')
    expect(roundTrip(md)).toContain('<summary>full output</summary>')
  })

  it('leaves dollar signs alone', () => {
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
    expect(commentValueToMarkdown(commentMarkdownToValue('')).trim()).toBe('')
  })
})
