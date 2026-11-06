import { describe, expect, it } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'

/** Types `text` a character at a time, the way the space rule sees a keystroke,
 *  and returns the urls of the links it made. */
function linksTyping(text: string) {
  const editor = createPlateEditor({ plugins: chatComposerPlugins, autoSelect: 'end' })
  for (const char of text) editor.tf.insertText(char)
  return [...editor.api.nodes({ at: [], match: { type: 'a' } })].map(
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
