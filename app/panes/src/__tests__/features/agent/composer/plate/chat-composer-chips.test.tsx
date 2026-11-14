import { createRef } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'

afterEach(cleanup)

async function draw(markdown: string) {
  const ref = createRef<ChatMarkdownEditorHandle>()
  const view = render(
    <ChatMarkdownEditor
      ref={ref}
      placeholder=""
      ariaLabel="Message the agent"
      onShapeChange={vi.fn()}
      onKeyDown={vi.fn()}
    />,
  )
  await act(async () => ref.current?.setMarkdown(markdown))
  return view.container
}

const chips = (root: HTMLElement, kind: 'slash' | 'ref') =>
  [...root.querySelectorAll(`[data-chip="${kind}"]`)].map((el) => el.textContent)

describe('the draft marks a slash command and a file reference as chips', () => {
  it('marks a command and an @path inside the text, leaving the text itself unchanged', async () => {
    const md = 'Start in @web/src/cart/store.ts then run /review on the diff.'
    const root = await draw(md)
    expect(chips(root, 'slash')).toEqual(['/review'])
    expect(chips(root, 'ref')).toEqual(['@web/src/cart/store.ts'])
    expect(root.textContent).toContain(md)
  })

  it('does not mark a slash inside a path or a URL', async () => {
    const root = await draw('see src/a/b.ts and https://x.dev/y/z')
    expect(chips(root, 'slash')).toEqual([])
  })

  it('marks nothing in a code block', async () => {
    const root = await draw('```\n/review @a/b.ts\n```')
    expect(chips(root, 'slash')).toEqual([])
    expect(chips(root, 'ref')).toEqual([])
  })
})
