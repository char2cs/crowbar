import { describe, expect, it } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { settleChatFreshText } from '@/features/agent/transcript/plate/chat-fresh-text-plugin'
import { applyStreamedValue } from '@/features/agent/transcript/plate/streaming-value-patch'

type FreshLeaf = { text: string; chatFresh?: number; chatFreshDelay?: number }

// `onAnimationEnd` itself isn't exercised here — jsdom ships no CSS engine, so
// no animation ever runs there to end (confirmed: even a bare React element's
// `onAnimationEnd` never fires under `fireEvent.animationEnd` in this suite).
// What's real Slate/Plate behavior, and what these test, is everything the
// handler actually calls: does settling a leaf correctly retire both marks
// and let it merge back to plain.
describe('settleChatFreshText', () => {
  it('unsets both marks and merges the leaf back into its plain neighbor', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building'),
    })
    applyStreamedValue(editor, chatMarkdownToValue('Building a CLI'))

    // The appended run splits per word (" a ", "CLI"), each with its own
    // generation/delay — see streaming-value-patch.ts.
    const leaves = editor.children[0]!.children as FreshLeaf[]
    expect(leaves.slice(1).every((leaf) => typeof leaf.chatFresh === 'number')).toBe(true)

    for (const leaf of leaves.slice(1)) settleChatFreshText(editor, leaf as never)

    // Same shape a fresh reparse would produce — one plain leaf, no seam.
    expect(editor.children).toEqual(chatMarkdownToValue('Building a CLI'))
  })

  it('is a safe no-op for a text node no longer in the document', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building a CLI'),
    })
    const orphan = { text: 'gone', chatFresh: 1, chatFreshDelay: 0 }

    expect(() => settleChatFreshText(editor, orphan as never)).not.toThrow()
    expect(editor.children).toEqual(chatMarkdownToValue('Building a CLI'))
  })
})
