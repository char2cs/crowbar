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

  // What a person types is what the agent reads: a character typed into a
  // paragraph is literal text, never markdown syntax to be escaped. The daemon
  // titles the chat from the prompt's first line, so the title inherits this.
  describe('literal characters typed into the box', () => {
    const literal = [
      'snake_case [x](y)',
      'rename snake_case_file.py to __init__.py',
      'a * b * c and 2*3*4',
      'use `foo` literally? no: a lone ` backtick',
      'issue #42 and # not a heading here',
      'compare a < b > c and <div> tags & entities',
      'open C:\\path\\to\\file and a\\b',
      '1. not a list when typed mid-paragraph? 1) either',
      '~~not struck~~ and !important and | pipes |',
    ]
    for (const text of literal) {
      it(`sends ${JSON.stringify(text)} unchanged`, () => {
        expect(chatValueToMarkdown([p(text)] as Value)).toBe(text)
      })
    }

    it('keeps the first line, which the chat is titled from, literal', () => {
      const out = chatValueToMarkdown([p('call set_chat_title [now]'), p('then *wait*')] as Value)
      expect(out.split('\n')[0]).toBe('call set_chat_title [now]')
    })

    it('still writes the markdown the editor holds as structure', () => {
      const value = [
        {
          type: 'p',
          children: [
            { text: 'fix ' },
            { text: 'my_var', code: true },
            { text: ' in ' },
            { text: 'snake_case', italic: true },
          ],
        },
        {
          type: 'code_block',
          lang: 'py',
          children: [{ type: 'code_line', children: [{ text: 'x = a_b * 2' }] }],
        },
      ] as Value
      expect(chatValueToMarkdown(value)).toBe(
        'fix `my_var` in *snake_case*\n\n```py\nx = a_b * 2\n```',
      )
    })

    it('round-trips a typed numbered list and fenced block', () => {
      expect(typed('1. first_step\n2. second [step]')).toBe('1. first_step\n2. second [step]')
      expect(typed('```\nsnake_case_file.py\n```')).toBe('```\nsnake_case_file.py\n```')
    })

    it('keeps a literal pipe from splitting a pasted table cell', () => {
      expect(typed('| a | b |\n| --- | --- |\n| x \\| y | z |')).toContain('x \\| y')
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
