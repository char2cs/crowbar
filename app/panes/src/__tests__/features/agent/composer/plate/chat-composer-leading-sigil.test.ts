import type { Value } from 'platejs'
import { describe, expect, it } from 'vitest'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { fileMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'

/**
 * The composer half of the leading-`!` shell-mode bug.
 *
 * An image attached BEFORE anything is typed serializes to `![alt](ref)`,
 * whose own first character is `!` — which both shipped CLIs read as "run the
 * rest as a shell command". Measured on codex-cli 0.149.1: the message never
 * reached the model, codex ran `[screenshot.png](/…/shot.png)` instead.
 *
 * The fix is NOT here. This string is also the durable ledger text and what
 * the sent bubble renders, so escaping it here would show the person a
 * backslash, or a link where their image was. The `!` is neutralised on the
 * DISPATCH COPY only, in Go — see runner.guardLeadingAttachmentSigil, which
 * recognises exactly the shapes pinned below.
 *
 * So these tests pin the CONTRACT that guard is written against: what the
 * composer hands the backend for an attachment sent first, and that a `!` a
 * person typed themselves is carried through untouched.
 */

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
    const md = chatValueToMarkdown(chatMarkdownToValue(fileMarkdown('notes.pdf', REF)))

    expect(md.startsWith('!')).toBe(false)
  })

  it('carries a leading `!` the person typed themselves through unchanged', () => {
    for (const typed of ['!ls -la', '!git status', '!', '!echo hi']) {
      expect(chatValueToMarkdown(chatMarkdownToValue(typed) as Value)).toBe(typed)
    }
  })
})
