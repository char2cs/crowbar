import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import type { Descendant, Value } from 'platejs'
import { describe, expect, it } from 'vitest'
import {
  parseMarkdown,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(parseMarkdown(md).value)

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
    const value = [
      { type: 'p', children: [{ text: 'plain ' }, { text: 'under', underline: true }] },
    ]
    expect(() => plateValueToMarkdown(value)).not.toThrow()
    expect(plateValueToMarkdown(value)).toContain('<u>under</u>')
  })

  it('deserializes <u> back into underlined text', () => {
    const md = 'This is <u>underlined</u> text.\n'
    const value = parseMarkdown(md).value
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
    const value = parseMarkdown(md).value
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
      const value = parseMarkdown(md).value
      expect(value).toMatchObject([{ type: 'callout', variant: type.toLowerCase() }])
      expect(roundTrip(md)).toContain(`[!${type}]`)
    }
  })

  it('leaves an ordinary blockquote alone', () => {
    const md = '> Just a quote.\n'
    const value = parseMarkdown(md).value
    expect(value).toMatchObject([{ type: 'blockquote' }])
  })
})

describe('markdown round-trip (mermaid)', () => {
  // A ```mermaid block stays an ordinary fenced code block in the document model: rendered
  // specially in the rich editor (mermaid-code-block.tsx) but round-tripped like any ```lang block.
  it('preserves a mermaid code fence byte-exactly', () => {
    const md = '```mermaid\nflowchart LR\n  A --> B\n```\n'
    expect(roundTrip(md)).toBe(md)
  })

  it('is genuinely a code_block node under the hood, not a special diagram node', () => {
    const md = '```mermaid\ngraph TD\n  A --> B\n```\n'
    const value = parseMarkdown(md).value
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

  // A mermaid block alongside a table and math must reach a fixed point too, so a pristine
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

// What a save writes: the value over the text it was parsed from.
const save = (md: string, edit: (value: Value) => Value = (value) => value) => {
  const { value, source } = parseMarkdown(md)
  return plateValueToMarkdown(edit(value), source)
}

// Appends text to the node at `index`, as typing at its end would.
const appendTo = (index: number, text: string) => (value: Value) =>
  value.map((node, i) =>
    i === index ? { ...node, children: [...node.children, { text }] } : node,
  ) as Value

// vitest runs from web/, so the repo root is one level up.
const REPO = join(process.cwd(), '..')
const repoDocs = [
  'README.md',
  'CLAUDE.md',
  'api/ARCHITECTURE.md',
  ...readdirSync(join(REPO, 'docs/plans'))
    .filter((name) => name.endsWith('.md'))
    .map((name) => `docs/plans/${name}`),
]

describe('markdown save over the source', () => {
  const samples: Record<string, string> = {
    'soft line breaks': 'first line\nsecond line\n',
    'hard breaks the user wrote': 'backslash\\\nbreak and two  \nspaces\n',
    'CRLF line endings': '# Title\r\n\r\nfirst\r\nsecond\r\n\r\n- a\r\n- b\r\n',
    'trailing whitespace and blank lines': 'para one   \n\n\n\npara two\n\n\n',
    'no final newline': 'last line',
    'setext headings': 'Title\n=====\n\nSub\n---\n',
    '_ emphasis, * bullets, nested emphasis':
      '_a_ and __b__ and ***c*** and *a **b** c*\n\n* one\n* two\n',
    'numbered list starting at 3': '3. three\n4. four\n\n1) paren\n',
    'indented and nested lists': '- a\n    - b\n        - c\n- d\n',
    'loose list': '- a\n\n- b\n',
    'tables with alignment': '|a|b|\n|:-|-:|\n|1|2|\n',
    'fences with languages':
      '```ts title=x\nconst a = 1\n```\n\n~~~\nplain\n~~~\n\n    indented code\n',
    'raw html': '<div align="center">\n  <img src="x.png">\n</div>\n\ntext <b>bold</b>\n',
    'escapes and tildes': 'a \\* star, ~30 ms, snake_case_name, 1\\. not a list\n',
    'links and references':
      '[ref][r] and <https://x.y> and [inline](u "t")\n\n[r]: https://example.com\n',
    'thematic breaks': 'a\n\n---\n\nb\n\n* * *\n',
    'blockquote with lazy line': '> quoted\nlazy\n',
  }

  for (const [name, md] of Object.entries(samples)) {
    it(`writes an untouched document back byte for byte: ${name}`, () => {
      expect(save(md)).toBe(md)
    })
  }

  it('writes every repo doc back byte for byte', () => {
    for (const path of repoDocs) {
      const md = readFileSync(join(REPO, path), 'utf8')
      expect(save(md), path).toBe(md)
    }
  })

  it('rewrites only the edited block; its neighbours keep their exact bytes', () => {
    const md = '_Intro_ with a soft\nbreak.\n\n\n| a |b|\n|---|---|\n| 1 |2|\n\nLast para.\n'
    expect(save(md, appendTo(2, ' more'))).toBe(
      '_Intro_ with a soft\nbreak.\n\n\n| a |b|\n|---|---|\n| 1 |2|\n\nLast para. more\n',
    )
  })

  it('keeps soft and hard line breaks apart in an edited paragraph', () => {
    expect(save('first line\nsecond line\n', appendTo(0, '!'))).toBe('first line\nsecond line!\n')
    expect(save('one\\\ntwo\nthree\n', appendTo(0, '!'))).toBe('one\\\ntwo\nthree!\n')
    expect(save('a `code`\n(paren)\n', appendTo(0, '!'))).toBe('a `code`\n(paren)!\n')
  })

  it('keeps a soft break inside emphasis', () => {
    expect(save('**bold\nacross** lines\n', appendTo(0, '!'))).toBe('**bold\nacross** lines!\n')
  })

  it('writes CRLF for an edited block of a CRLF document', () => {
    expect(save('a\r\nb\r\n\r\nc\r\n', appendTo(1, '!'))).toBe('a\r\nb\r\n\r\nc!\r\n')
  })

  it('drops a deleted block with its spacing and keeps the rest', () => {
    const md = '# Title\n\nGone.\n\n* kept\n'
    expect(save(md, (value) => value.filter((_, i) => i !== 1) as Value)).toBe(
      '# Title\n\n* kept\n',
    )
  })

  it('keeps a link definition no node represents', () => {
    const md = 'See [x][r].\n\n[r]: https://example.com\n\nEnd.\n'
    expect(save(md, appendTo(2, '!'))).toContain('[r]: https://example.com')
  })

  it('keeps a list one list when an item is added to it', () => {
    const md = 'Intro.\n\n- a\n- b\n'
    const out = save(md, (value) => [...value, { ...value[2], children: [{ text: 'c' }] }] as Value)
    expect(out).toBe('Intro.\n\n- a\n- b\n- c\n')
  })

  it('serializes inserted blocks between kept ones', () => {
    const md = 'one\n\ntwo\n'
    const inserted: Descendant = { type: 'p', children: [{ text: 'new' }] }
    expect(save(md, (value) => [value[0], inserted, value[1]] as Value)).toBe('one\n\nnew\n\ntwo\n')
  })

  it('writes no empty paragraph, which markdown cannot hold', () => {
    const empty: Descendant = { type: 'p', children: [{ text: '' }] }
    expect(save('one\n\ntwo\n', (value) => [value[0], empty, value[1]] as Value)).toBe(
      'one\n\ntwo\n',
    )
    expect(save('', () => [empty])).toBe('')
  })
})
