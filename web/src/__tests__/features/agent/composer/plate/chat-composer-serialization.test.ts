import type { Value } from 'platejs'
import { describe, expect, it } from 'vitest'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'

/** A paragraph, as the editor holds it — a round trip cannot express one that
    still carries the whitespace the parser strips. */
const p = (text: string) => ({ type: 'p', children: [{ text }] })

/** What the box holds after someone types `text` and stops. */
function typed(text: string) {
  return chatValueToMarkdown(chatMarkdownToValue(text))
}

type Doc = { text?: string; children?: Doc[]; [key: string]: unknown }

/** Slate may split or pad text leaves anywhere; joining runs of identical marks
    leaves only what a reader can see. */
function sameText(nodes: Value): Doc[] {
  const out: Doc[] = []
  for (const node of nodes as Doc[]) {
    if (node.children) {
      out.push({ ...node, children: sameText(node.children as Value) })
      continue
    }
    const { text = '', ...marks } = node
    const prev = out.at(-1)
    const prevMarks = prev && prev.text !== undefined ? { ...prev, text: undefined } : undefined
    if (
      prev &&
      prevMarks &&
      JSON.stringify(prevMarks) === JSON.stringify({ ...marks, text: undefined })
    )
      prev.text += text
    else if (text !== '') out.push({ ...node })
  }
  return out
}

describe('the chat composer codec', () => {
  it('carries the markdown a prompt is written in', () => {
    expect(typed('Fix **this** and the `parser`')).toBe('Fix **this** and the `parser`')
    expect(typed('- one\n- two')).toBe('- one\n- two')
    expect(typed('# Heading')).toBe('# Heading')
    expect(typed('> quoted')).toBe('> quoted')
  })

  // REGRESSION: remark-stringify PRESERVES outer whitespace by escaping it, so a
  // prompt typed with a trailing space — most of them — reached the model as
  // `…just with Codex?&#x20;`. Two real prompts were sent that way before this
  // was caught.
  it('never encodes trailing whitespace into the prompt', () => {
    expect(typed('what were we talking about? ')).toBe('what were we talking about?')
    expect(typed('name this chat please. ')).toBe('name this chat please.')
    expect(typed('  padded  ')).toBe('padded')
    expect(typed('trailing newline\n')).toBe('trailing newline')
  })

  // REGRESSION, the second half of the same bug. The escape is per LINE, not per
  // document, so trimming the finished string cleared only its two outer ends —
  // every interior line edge still shipped encoded, and a two-line prompt (the
  // Shift+Enter case) reached the model as `hello&#x20;\n\nworld`.
  describe('whitespace at the edge of an interior line', () => {
    const held = {
      'a paragraph that trails, then another': [p('hello '), p('world')],
      'a paragraph with a leading space': [p('hello'), p('  world')],
      'two trailing spaces, which would be a hard break': [p('hello  '), p('world')],
      'a newline held inside one leaf': [p('line one \nline two')],
      'a heading that trails': [{ type: 'h1', children: [{ text: 'Title ' }] }, p('body')],
      'a blockquote that trails': [{ type: 'blockquote', children: [p('quoted ')] }],
    } satisfies Record<string, Value>

    for (const [what, value] of Object.entries(held)) {
      it(`is cleared on ${what}`, () => {
        expect(chatValueToMarkdown(value as Value)).not.toContain('&#x20;')
      })
    }

    it('leaves the live editor document alone', () => {
      const value = [p('hello ')] as Value
      chatValueToMarkdown(value)
      expect(value).toEqual([p('hello ')])
    })
  })

  // REGRESSION: a genuinely empty document — a fresh, untouched box, or one
  // typed into and then cleared back out — normalizes (Slate cannot hold zero
  // children) to a single empty paragraph. The underlying codec serializes
  // THAT as a lone U+200B (zero-width space), its own placeholder for telling
  // an intentionally-blank paragraph apart from no paragraph at all on a
  // markdown round trip. `.trim()` does not touch it — U+200B is a Cf
  // formatting character, not whitespace — so this reached `isPromptTextWithinLimit`
  // as one "real" character and a box that looked, and read via .textContent,
  // completely empty could be sent as a real turn.
  describe('a document with nothing actually typed in it', () => {
    it('serializes a single empty paragraph to the empty string, not U+200B', () => {
      expect(chatValueToMarkdown([p('')] as Value)).toBe('')
    })

    it('serializes several empty paragraphs (Enter pressed with nothing typed) to empty', () => {
      expect(chatValueToMarkdown([p(''), p('')] as Value)).toBe('')
    })

    it('treats a whitespace-only paragraph the same way', () => {
      expect(chatValueToMarkdown([p('   ')] as Value)).toBe('')
    })

    it('never leaves a stray U+200B in real content around an empty paragraph', () => {
      expect(chatValueToMarkdown([p('hello'), p('')] as Value)).toBe('hello')
    })
  })

  // The draft, stash, queue and history store this markdown and re-parse it,
  // and the transcript bubble renders it, so a prompt must come back as the
  // document it was. Within that, escapes the parser does not need are dropped.
  describe('a prompt read back from what was sent', () => {
    const typedTexts = [
      'snake_case [x](y)',
      'rename snake_case_file.py to __init__.py',
      'a * b * c and 2*3*4',
      'use `foo` literally? no: a lone ` backtick',
      'issue #42 and # not a heading here',
      'compare a < b > c and <div> tags & entities',
      'open C:\\path\\to\\file and a\\b',
      '1. not a list when typed mid-paragraph? 1) either',
      '~~not struck~~ and !important and | pipes |',
      '_a_b_',
      '__init__.py',
      'escape \\_ and \\* and \\[',
      '&amp; &lt;b&gt;',
      '---',
      '+ plus',
      '~~~',
      'regex ^\\d+\\.\\s*$',
      'C:\\Users\\me\\_dir',
      'x = a_b * c_d * e',
      '*args and **kwargs',
      'def f(*args, **kwargs)',
      'a <!-- c --> b',
      '1) item',
      '* star bullet',
      '#hashtag',
      '<script>x</script>',
      'tab\tinside',
      'trail\\',
      'line\\\nnext',
      'café ñ 😀 中文',
      '[ref]: http://x',
      '[a][b]',
      'call set_chat_title [now]',
    ]
    const leaf = (text: string, marks: Record<string, boolean> = {}) => ({ text, ...marks })
    const structured: Record<string, Value> = {
      'link text with a bracket': [
        {
          type: 'p',
          children: [
            leaf('see '),
            { type: 'a', url: 'http://u', children: [leaf('a]b')] },
            leaf(''),
          ],
        },
      ],
      'italic ending in a backslash': [
        { type: 'p', children: [leaf('x '), leaf('dir\\', { italic: true }), leaf(' y')] },
      ],
      'a star beside bold': [{ type: 'p', children: [leaf('foo*'), leaf('bar', { bold: true })] }],
      'a backtick beside code': [{ type: 'p', children: [leaf('a`'), leaf('b', { code: true })] }],
      'a list item starting with a dash': [
        { type: 'p', indent: 1, listStyleType: 'disc', children: [leaf('- inner')] },
      ],
      'a heading starting with a hash': [{ type: 'h2', children: [leaf('# hash')] }],
      'a quote starting with >': [{ type: 'blockquote', children: [p('> nested?')] }],
      'an indented code line': [
        {
          type: 'code_block',
          children: [{ type: 'code_line', children: [leaf('    if a_b: x = c*d')] }],
        },
      ],
      'table cells holding | and \\|': [
        {
          type: 'table',
          children: [
            {
              type: 'tr',
              children: [
                { type: 'th', children: [p('h|1')] },
                { type: 'th', children: [p('h2')] },
              ],
            },
            {
              type: 'tr',
              children: [
                { type: 'td', children: [p('a|b')] },
                { type: 'td', children: [p('c\\|d')] },
              ],
            },
          ],
        },
      ],
    } as Record<string, Value>
    const cases: [string, Value][] = [
      ...typedTexts.map((text): [string, Value] => [JSON.stringify(text), [p(text)] as Value]),
      ...Object.entries(structured),
    ]

    for (const [name, value] of cases) {
      it(`comes back unchanged: ${name}`, () => {
        expect(sameText(chatMarkdownToValue(chatValueToMarkdown(value)))).toEqual(sameText(value))
      })
    }

    it('sends a name, path or list marker without escapes the parser does not need', () => {
      const sent = (text: string) => chatValueToMarkdown([p(text)] as Value)
      expect(sent('snake_case_file.py and set_chat_title')).toBe(
        'snake_case_file.py and set_chat_title',
      )
      expect(sent('a * b and 2*3 or a_b * c_d')).toBe('a * b and 2*3 or a_b * c_d')
      expect(sent('open C:\\path\\to\\file')).toBe('open C:\\path\\to\\file')
      expect(sent('compare a < b > c, #42 & ~ ! |')).toBe('compare a < b > c, #42 & ~ ! |')
    })

    it('still writes the markdown the editor holds as structure', () => {
      const value = [
        {
          type: 'p',
          children: [
            leaf('fix '),
            leaf('my_var', { code: true }),
            leaf(' in '),
            leaf('snake_case', { italic: true }),
          ],
        },
        {
          type: 'code_block',
          lang: 'py',
          children: [{ type: 'code_line', children: [leaf('x = a_b * 2')] }],
        },
      ] as Value
      expect(chatValueToMarkdown(value)).toBe(
        'fix `my_var` in *snake_case*\n\n```py\nx = a_b * 2\n```',
      )
    })
  })

  // A code line's indentation IS the program. It is the one place the encoder
  // never runs and the one place trimming would corrupt the prompt.
  it('keeps the indentation inside a fenced block', () => {
    const out = chatValueToMarkdown(chatMarkdownToValue('```go\nif x {\n  return y\n}\n```'))
    expect(out).toContain('\n  return y')
  })

  // A table pasted into the box has to reach the agent as a table: an
  // unregistered node is dropped by @platejs/markdown, silently.
  it('keeps a pasted table rather than dropping it', () => {
    const table = '| a | b |\n| --- | --- |\n| 1 | 2 |'
    const out = typed(table)
    expect(out).toContain('| a')
    expect(out).toContain('| 1')
  })

  it('keeps a fenced code block, language and all', () => {
    const out = typed('```go\nfunc main() {}\n```')
    expect(out).toContain('```go')
    expect(out).toContain('func main() {}')
  })
})
