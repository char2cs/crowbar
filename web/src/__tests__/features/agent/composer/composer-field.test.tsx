import { createRef } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ComposerField } from '@/features/agent/composer/composer-field'
import type { ChatMarkdownEditorHandle } from '@/features/agent/composer/plate/chat-markdown-editor'

const ref = createRef<ChatMarkdownEditorHandle>()

function drawField(chatId: string) {
  render(
    <ComposerField
      ref={ref}
      wsId="w1"
      chatId={chatId}
      placement="dock"
      placeholder=""
      expanded={false}
      onShapeChange={vi.fn()}
      onKeyDown={vi.fn()}
      onHeightChange={vi.fn()}
    />,
  )
}

// What the page held as it went: the box is drawn again from nothing.
function reload(chatId: string) {
  act(() => void window.dispatchEvent(new Event('pagehide')))
  cleanup()
  drawField(chatId)
}

describe('ComposerField — a chat keeps its unsent draft', () => {
  it('gives the words back after a reload, to that chat only', () => {
    drawField('c1')
    act(() => ref.current?.setMarkdown('half a thought'))

    reload('c2')
    expect(ref.current?.getMarkdown()).toBe('')
    reload('c1')
    expect(ref.current?.getMarkdown().trim()).toBe('half a thought')
  })

  it('forgets the draft once a send clears the box', () => {
    drawField('c3')
    act(() => ref.current?.setMarkdown('about to send'))
    act(() => ref.current?.clear())

    reload('c3')
    expect(ref.current?.getMarkdown()).toBe('')
  })
})
