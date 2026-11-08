import { describe, expect, it } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { MarkdownPlugin } from '@platejs/markdown'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'

/** An editor with `text` typed a character at a time, the way input rules see keystrokes. */
function typed(text: string) {
  const editor = createPlateEditor({ plugins: chatComposerPlugins, autoSelect: 'end' })
  for (const char of text) editor.tf.insertText(char)
  return editor
}

/** The urls of the links typing `text` made. */
function linksTyping(text: string) {
  return [...typed(text).api.nodes({ at: [], match: { type: 'a' } })].map(
    ([node]) => (node as { url?: string }).url,
  )
}

describe('ChatLinkKit autolink', () => {
  it('links a typed address when a space follows it', () => {
    expect(linksTyping('see https://example.com/a next')).toEqual(['https://example.com/a'])
  })

  it('never takes the punctuation that ends a sentence into the link', () => {
    expect(linksTyping('see https://example.com/a; next')).not.toContain('https://example.com/a;')
    expect(linksTyping('see https://example.com/a. next')).not.toContain('https://example.com/a.')
  })
})

describe('ChatLinkKit autolink in code', () => {
  // A code span cannot hold a link: linking inside one splits it when it closes.
  it('sends inline code holding an address exactly as typed', () => {
    const editor = typed('run `curl -sI https://example.com | head -1` now')
    expect(editor.getApi(MarkdownPlugin).markdown.serialize().trim()).toBe(
      'run `curl -sI https://example.com | head -1` now',
    )
  })

  it('leaves an address typed inside a code mark unlinked', () => {
    const editor = createPlateEditor({ plugins: chatComposerPlugins, autoSelect: 'end' })
    editor.tf.toggleMark('code')
    for (const char of 'https://example.com next') editor.tf.insertText(char)
    expect([...editor.api.nodes({ at: [], match: { type: 'a' } })]).toEqual([])
  })
})
