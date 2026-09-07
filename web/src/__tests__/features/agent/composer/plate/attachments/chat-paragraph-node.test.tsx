import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useDropLine } from '@platejs/dnd'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

// Same mocked-boundary approach as attachment-drag-handle.test.tsx: only
// `useDropLine` (the drop-line INDICATOR's own data source) is mocked here —
// `useAttachmentDropTarget`'s own `DndContext`-presence guard is exercised
// for real, since that is exactly what has to hold with no `<DndProvider>`
// ancestor (every test below renders with none).
vi.mock('@platejs/dnd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@platejs/dnd')>()
  return { ...actual, useDropLine: vi.fn().mockReturnValue({ dropLine: undefined }) }
})

describe('ChatParagraphElement', () => {
  it('renders a plain paragraph normally, with no DndProvider ancestor at all', () => {
    expect(() => render(<MarkdownMessage>{'just some text'}</MarkdownMessage>)).not.toThrow()
    expect(screen.getByText('just some text')).toBeInTheDocument()
  })

  it('renders a drop-line bar when useDropLine reports one hovering this paragraph', () => {
    vi.mocked(useDropLine).mockReturnValue({ dropLine: 'top' })
    render(<MarkdownMessage>{'just some text'}</MarkdownMessage>)
    const paragraph = screen.getByText('just some text').closest('.slate-p')
    expect(paragraph).not.toBeNull()
    expect(paragraph!.querySelector('.-top-px')).not.toBeNull()
  })
})

// The static/settled variant keeps the plain, shared `ParagraphElement` —
// registered on `chatComposerPluginsStatic`, confirmed here rather than
// assumed: it must never call `useDropLine` (mocked above to prove the
// negative — if it were called, that mock's last `dropLine: 'top'` value
// from the test above would render a bar here too).
describe('static paragraph rendering', () => {
  it('never renders a drop-line bar for a settled message', () => {
    vi.mocked(useDropLine).mockReturnValue({ dropLine: 'top' })
    render(<MarkdownMessageStatic>{'just some text'}</MarkdownMessageStatic>)
    const paragraph = screen.getByText('just some text').closest('.slate-p')
    expect(paragraph).not.toBeNull()
    expect(paragraph!.querySelector('.-top-px')).toBeNull()
  })
})
