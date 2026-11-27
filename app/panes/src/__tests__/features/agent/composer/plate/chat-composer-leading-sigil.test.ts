import type { Value } from 'platejs'
import { describe, expect, it } from 'vitest'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'

const REF = 'chats/chat-1/attachments/ab12-screenshot.png'

describe('TestRegression_ComposerLeadingSigil', () => {
  it('hands an attachment sent first over as the exact shape the dispatch guard matches', () => {
    const md = chatValueToMarkdown(chatMarkdownToValue(imageMarkdown('screenshot.png', REF)))

    expect(md).toBe(`![screenshot.png](${REF})`)
    expect(md.startsWith('![')).toBe(true)
  })

  it('keeps that shape leading when the person types after attaching', () => {
    const md = chatValueToMarkdown(
      chatMarkdownToValue(`${imageMarkdown('screenshot.png', REF)}\n\nwhat is wrong here?`),
    )

    expect(md).toBe(`![screenshot.png](${REF})\n\nwhat is wrong here?`)
  })

  it('never opens with a sigil for a non-image attachment, which is a plain link', () => {
    const md = chatValueToMarkdown(chatMarkdownToValue(`[notes.pdf](${REF})`))

    expect(md.startsWith('!')).toBe(false)
  })

  it('carries a leading `!` the person typed themselves through unchanged', () => {
    for (const typed of ['!ls -la', '!git status', '!', '!echo hi']) {
      expect(chatValueToMarkdown(chatMarkdownToValue(typed) as Value)).toBe(typed)
    }
  })
})
