import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DndProvider } from 'react-dnd'
import { HTML5Backend } from 'react-dnd-html5-backend'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

afterEach(cleanup)

/** Same reasoning as chat-code-block-node.test.tsx's own `renderWithDnd` —
 *  `@platejs/dnd`'s `useDraggable` throws "Expected drag drop context"
 *  without a real `<DndProvider>` ancestor once `DndPlugin` is registered. */
function renderWithDnd(ui: React.ReactNode) {
  return render(<DndProvider backend={HTML5Backend}>{ui}</DndProvider>)
}

describe('chat attachment images', () => {
  it('renders a plain image normally, unchanged from before this task', () => {
    render(<MarkdownMessageStatic>{'![a photo](https://example.com/photo.png)'}</MarkdownMessageStatic>)
    const img = screen.getByAltText('a photo')
    expect(img).toBeInTheDocument()
  })

  // REGRESSION, reported live: images had no max-height cap at all (unlike
  // every other attachment kind — the excalidraw preview, the text-attachment
  // pill), so a tall photo could take over the whole composer.
  it('caps a settled image at a fixed max height', () => {
    render(<MarkdownMessageStatic>{'![a photo](https://example.com/photo.png)'}</MarkdownMessageStatic>)
    const img = screen.getByAltText('a photo')
    expect(img.className).toContain('max-h-80')
  })

  it('caps an interactive (composer) image at the same fixed max height', () => {
    renderWithDnd(<MarkdownMessage>{'![a photo](https://example.com/photo.png)'}</MarkdownMessage>)
    const img = screen.getByAltText('a photo')
    expect(img.className).toContain('max-h-80')
  })

  // REGRESSION, reported live: "Images ... cannot be rearranged or reordered
  // once on the input box" — every other attachment kind got a drag handle;
  // images never did.
  it('renders a drag handle for an interactive image, unlike a settled one', () => {
    const interactive = renderWithDnd(
      <MarkdownMessage>{'![a photo](https://example.com/photo.png)'}</MarkdownMessage>,
    )
    expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()
    interactive.unmount()

    render(<MarkdownMessageStatic>{'![a photo](https://example.com/photo.png)'}</MarkdownMessageStatic>)
    expect(screen.queryByRole('button', { name: /reorder this attachment/i })).toBeNull()
  })

  it('renders a delete button for an interactive image, which removes it on click', async () => {
    renderWithDnd(<MarkdownMessage>{'![a photo](https://example.com/photo.png)'}</MarkdownMessage>)
    expect(screen.getByRole('button', { name: /remove this attachment/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))

    await vi.waitFor(() => expect(screen.queryByAltText('a photo')).toBeNull())
  })

  // The excalidraw PNG sibling hide (chat-markdown-image-node.tsx, pre-
  // existing behavior) must survive both the max-height and drag additions —
  // it's still a completely different render branch (a `hidden` PlateElement,
  // no `<img>` at all).
  it('still hides the excalidraw PNG sibling, with no drag handle either', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    renderWithDnd(<MarkdownMessage>{md}</MarkdownMessage>)

    expect(document.querySelectorAll('img')).toHaveLength(0)
    // Only the excalidraw fence's OWN handle, none for the hidden PNG.
    expect(screen.getAllByRole('button', { name: /reorder this attachment/i })).toHaveLength(1)
  })
})
