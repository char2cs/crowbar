import { createRef } from 'react'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposerField } from '@/features/agent/composer/composer-field'
import type { ChatMarkdownEditorHandle } from '@/features/agent/composer/plate/chat-markdown-editor'

const ref = createRef<ChatMarkdownEditorHandle>()

const onShapeChange = vi.fn()

function drawField(
  chatId: string,
  placement: 'document' | 'dock' = 'dock',
  onKeyDown: () => void = vi.fn(),
) {
  render(
    <ComposerField
      ref={ref}
      wsId="w1"
      chatId={chatId}
      placement={placement}
      placeholder=""
      expanded={false}
      onShapeChange={onShapeChange}
      onKeyDown={onKeyDown}
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

describe('ComposerField — Enter', () => {
  afterEach(cleanup)

  const enter = (init: { metaKey?: boolean; ctrlKey?: boolean } = {}) => {
    const box = screen.getByRole('textbox')
    // jsdom does not implement contentEditable; Slate checks it before dispatching a key.
    Object.defineProperty(box, 'isContentEditable', { value: true, configurable: true })
    act(() => box.focus())
    fireEvent.keyDown(box, { key: 'Enter', ...init })
  }

  it('goes to the chat in the pill, which sends on it', () => {
    const onKeyDown = vi.fn()
    drawField('e1', 'dock', onKeyDown)
    enter()
    expect(onKeyDown).toHaveBeenCalledTimes(1)
  })

  it('is a new line in the document: the chat never sees it', () => {
    const onKeyDown = vi.fn()
    drawField('e2', 'document', onKeyDown)
    enter()
    expect(onKeyDown).not.toHaveBeenCalled()
  })

  it.each([{ metaKey: true }, { ctrlKey: true }])(
    'sends from the document on %o + Enter',
    (mod) => {
      const onKeyDown = vi.fn()
      drawField('e3', 'document', onKeyDown)
      enter(mod)
      expect(onKeyDown).toHaveBeenCalledTimes(1)
    },
  )

  it('hands an open command to the picker even in the document', () => {
    const onKeyDown = vi.fn()
    drawField('e4', 'document', onKeyDown)
    act(() => ref.current?.startCommand())
    enter()
    expect(onKeyDown).toHaveBeenCalledTimes(1)
  })
})
