import type { Value } from 'platejs'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as serialization from '@/features/agent/composer/plate/chat-composer-serialization'
import { chatPromptToSend } from '@/features/agent/composer/plate/chat-prompt-send'
import { leaf, promptCases, sameText } from '@/__tests__/__fixtures__/composer-prompts'

const { chatMarkdownToValue, chatValueToMarkdown } = serialization
const p = (text: string) => ({ type: 'p', children: [{ text }] })
const send = (value: Value) => chatPromptToSend(chatValueToMarkdown(value))

afterEach(() => vi.restoreAllMocks())

describe('chatPromptToSend', () => {
  // The bubble, the queue row and history recall all re-parse what was sent.
  for (const [name, value] of promptCases) {
    it(`sends a prompt that reads back unchanged: ${name}`, () => {
      const sent = send(value)
      const back = chatMarkdownToValue(sent)
      expect(sameText(back)).toEqual(sameText(value))
      expect(send(back)).toBe(sent)
    })
  }

  it('sends a name, path or operator without escapes the parser does not need', () => {
    const sent = (text: string) => send([p(text)] as Value)
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
    expect(send(value)).toBe('fix `my_var` in *snake_case*\n\n```py\nx = a_b * 2\n```')
  })

  it('sends a prompt too long to re-parse as serialized, escapes and all', () => {
    const value = [p(Array(1200).fill('a_b').join(' '))] as Value
    const draft = chatValueToMarkdown(value)
    expect(draft).toContain('a\\_b')
    expect(chatPromptToSend(draft)).toBe(draft)
    expect(sameText(chatMarkdownToValue(draft))).toEqual(sameText(value))
  })

  it('parses a prompt once however often it is asked for', () => {
    const draft = chatValueToMarkdown([p('cached snake_case and __init__.py')] as Value)
    const parse = vi.spyOn(serialization, 'chatMarkdownToValue')
    const first = chatPromptToSend(draft)
    const parses = parse.mock.calls.length
    expect(parses).toBeGreaterThan(0)
    expect(chatPromptToSend(draft)).toBe(first)
    expect(parse.mock.calls.length).toBe(parses)
  })

  // The draft is serialized on every keystroke; the re-parses are paid at send.
  it('leaves the draft escaped and parses nothing while typing', () => {
    const parse = vi.spyOn(serialization, 'chatMarkdownToValue')
    expect(chatValueToMarkdown([p('typing snake_case [x](y)')] as Value)).toBe(
      'typing snake\\_case \\[x]\\(y)',
    )
    expect(parse).not.toHaveBeenCalled()
  })
})
