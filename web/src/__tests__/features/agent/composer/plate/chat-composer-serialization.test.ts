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
