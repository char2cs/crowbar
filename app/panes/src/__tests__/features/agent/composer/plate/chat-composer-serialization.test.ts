import type { Value } from 'platejs'
import { describe, expect, it } from 'vitest'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { promptCases, sameText } from '@/__tests__/__fixtures__/composer-prompts'

const p = (text: string) => ({ type: 'p', children: [{ text }] })

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

  it('never encodes trailing whitespace into the prompt', () => {
    expect(typed('what were we talking about? ')).toBe('what were we talking about?')
    expect(typed('name this chat please. ')).toBe('name this chat please.')
    expect(typed('  padded  ')).toBe('padded')
    expect(typed('trailing newline\n')).toBe('trailing newline')
  })

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

  for (const [name, value] of promptCases) {
    it(`reads a draft back unchanged: ${name}`, () => {
      expect(sameText(chatMarkdownToValue(chatValueToMarkdown(value)))).toEqual(sameText(value))
    })
  }

  it('keeps the indentation inside a fenced block', () => {
    const out = chatValueToMarkdown(chatMarkdownToValue('```go\nif x {\n  return y\n}\n```'))
    expect(out).toContain('\n  return y')
  })

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
