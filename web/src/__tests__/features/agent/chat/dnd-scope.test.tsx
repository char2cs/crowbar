import { createRef, StrictMode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'

const FENCE = '```text-attachment:AbC123xy\nsome long pasted text\n```'

/** jsdom has no layout: give the three blocks of the document stacked rects
 *  so dnd-kit's collision detection has something to measure. */
function stubLayout(rects: [Element, number, number][]) {
  const original = Element.prototype.getBoundingClientRect
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    const hit = rects.find(([el]) => el === this || el.contains(this))
    if (!hit) return original.call(this)
    const [, top, height] = hit
    return DOMRect.fromRect({ x: 0, y: top, width: 400, height })
  })
}

async function renderDocument() {
  const ref = createRef<ChatMarkdownEditorHandle>()
  const onChange = vi.fn()
  render(
    <TooltipProvider>
      <DndScope>
        <ChatMarkdownEditor
          ref={ref}
          initialValue="first paragraph"
          placeholder=""
          ariaLabel="Message the agent"
          onChange={onChange}
          onKeyDown={vi.fn()}
        />
      </DndScope>
    </TooltipProvider>,
  )
  await act(async () => {
    ref.current?.insertAttachmentMarkdown(FENCE)
    ref.current?.insertAttachmentMarkdown('second paragraph')
  })
  const handle = await screen.findByRole('button', { name: /reorder this attachment/i })
  const first = screen.getByText('first paragraph').closest('.slate-p')!
  const block = handle.closest('.slate-code_block')!
  const second = screen.getByText('second paragraph').closest('.slate-p')!
  stubLayout([
    [first, 0, 20],
    [block, 20, 40],
    [second, 60, 20],
  ])
  return { handle, block, second, onChange }
}

// The drop selects the moved block, which opens the floating toolbar; it
// positions itself off the DOM selection's rect, which jsdom's Range lacks.
beforeEach(() => {
  Range.prototype.getBoundingClientRect ??= () => DOMRect.fromRect()
  Range.prototype.getClientRects ??= () => [] as unknown as DOMRectList
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('DndScope', () => {
  it('mounts several scopes at once, under StrictMode, without throwing', () => {
    expect(() =>
      render(
        <StrictMode>
          <DndScope>{null}</DndScope>
          <DndScope>{null}</DndScope>
        </StrictMode>,
      ),
    ).not.toThrow()
  })

  it('drags an attachment by its handle below a later paragraph: drop line while dragging, moved on release', async () => {
    const { handle, block, second, onChange } = await renderDocument()

    fireEvent.pointerDown(handle, { clientX: 5, clientY: 30, button: 0, isPrimary: true })
    fireEvent.pointerMove(document, { clientX: 5, clientY: 70 })
    fireEvent.pointerMove(document, { clientX: 5, clientY: 75 })

    expect(block.className).toContain('opacity-50')
    expect(second.querySelector('.-bottom-px')).not.toBeNull()

    fireEvent.pointerUp(document, { clientX: 5, clientY: 75 })

    await waitFor(() => {
      const markdown = onChange.mock.calls.at(-1)?.[0] as string
      expect(markdown.indexOf('second paragraph')).toBeLessThan(markdown.indexOf('text-attachment'))
    })
    expect(document.querySelector('.-top-px, .-bottom-px')).toBeNull()
    expect(document.querySelector('.slate-code_block')?.className).not.toContain('opacity-50')
  })

  it('does not move the attachment when released where it already is', async () => {
    const { handle, second, onChange } = await renderDocument()
    const calls = onChange.mock.calls.length

    fireEvent.pointerDown(handle, { clientX: 5, clientY: 30, button: 0, isPrimary: true })
    // Top half of the paragraph right below the attachment: a no-op drop.
    fireEvent.pointerMove(document, { clientX: 5, clientY: 62 })
    expect(second.querySelector('.-top-px, .-bottom-px')).toBeNull()
    fireEvent.pointerUp(document, { clientX: 5, clientY: 62 })

    expect(onChange.mock.calls.length).toBe(calls)
  })

  it('treats a press without movement as a click, not a drag', async () => {
    const { handle, block } = await renderDocument()

    fireEvent.pointerDown(handle, { clientX: 5, clientY: 30, button: 0, isPrimary: true })
    fireEvent.pointerMove(document, { clientX: 6, clientY: 31 })

    expect(block.className).not.toContain('opacity-50')
    fireEvent.pointerUp(document, { clientX: 6, clientY: 31 })
  })
})
