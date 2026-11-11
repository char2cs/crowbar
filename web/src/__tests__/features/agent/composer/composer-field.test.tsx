import { createRef } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ComposerField } from '@/features/agent/composer/composer-field'
import type { ChatMarkdownEditorHandle } from '@/features/agent/composer/plate/chat-markdown-editor'

const ref = createRef<ChatMarkdownEditorHandle>()

const onShapeChange = vi.fn()

function drawField(chatId: string) {
  render(
    <ComposerField
      ref={ref}
      wsId="w1"
      chatId={chatId}
      placement="dock"
      placeholder=""
      expanded={false}
      onShapeChange={onShapeChange}
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

describe('ComposerField — an @ path', () => {
  it('reports the @ path before the caret, and completes it in place', () => {
    drawField('c4')
    act(() => ref.current?.setMarkdown('see @src/ma'))
    expect(onShapeChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ mentionQuery: 'src/ma' }),
      false,
    )

    act(() => ref.current?.completeMention('@src/main.ts '))
    expect(ref.current?.getMarkdown().trim()).toBe('see @src/main.ts')
    expect(onShapeChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ mentionQuery: null }),
      true,
    )
  })
})

describe('ComposerField — a command', () => {
  it('starts one in an empty box with a bare slash', () => {
    drawField('c5')
    act(() => ref.current?.startCommand())

    expect(ref.current?.getMarkdown().trim()).toBe('/')
    expect(onShapeChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ slashQuery: '' }),
      true,
    )
  })

  it('starts one above a draft, which stays under it', () => {
    drawField('c6')
    act(() => ref.current?.setMarkdown('fix the rounding'))
    act(() => ref.current?.startCommand())

    expect(ref.current?.getMarkdown().trim()).toBe('/\n\nfix the rounding')
    expect(onShapeChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ slashQuery: '' }),
      true,
    )
  })

  it('completes the command line and leaves the lines below it alone', () => {
    drawField('c7')
    act(() => ref.current?.setMarkdown('/he\n\nfix the rounding'))
    act(() => ref.current?.completeCommand('/help '))

    expect(ref.current?.getMarkdown().trim()).toBe('/help\n\nfix the rounding')
  })
})
