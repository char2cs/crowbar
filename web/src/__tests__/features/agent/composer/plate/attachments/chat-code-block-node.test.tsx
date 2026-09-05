import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import * as ExcalidrawPreviewModule from '@/features/agent/composer/plate/attachments/excalidraw-preview'

// Wraps the REAL component (not a stand-in) so every existing behavioural
// assertion below still exercises actual rendering — this only adds the
// ability to inspect the `pngRef` prop `ChatCodeBlockElement` computed for
// it, which two tests below need to prove the sibling-`img`-lookup branches
// (found vs. not-found) without duplicating ExcalidrawPreview's own
// pngRef-resolution tests (see excalidraw-preview.test.tsx).
vi.mock(
  '@/features/agent/composer/plate/attachments/excalidraw-preview',
  async (importOriginal) => {
    const actual =
      await importOriginal<
        typeof import('@/features/agent/composer/plate/attachments/excalidraw-preview')
      >()
    return { ...actual, ExcalidrawPreview: vi.fn(actual.ExcalidrawPreview) }
  },
)

describe('chat attachment code blocks', () => {
  it('renders a plain code block for a bare ```text-attachment fence with no id', () => {
    render(
      <MarkdownMessageStatic>
        {'```text-attachment\njust discussing the feature\n```'}
      </MarkdownMessageStatic>,
    )
    expect(screen.getByText('just discussing the feature')).toBeInTheDocument()
    expect(screen.queryByText(/pasted text/i)).toBeNull()
  })

  it('renders a plain code block for a completely unrelated language tag, identical to before', () => {
    render(<MarkdownMessageStatic>{'```go\nfunc main() {}\n```'}</MarkdownMessageStatic>)
    expect(document.querySelector('pre')).not.toBeNull()
    expect(document.querySelector('pre')?.textContent).toContain('func main')
    expect(screen.queryByText(/pasted text/i)).toBeNull()
    expect(screen.queryByText(/excalidraw diagram/i)).toBeNull()
  })

  it('renders a pill for a fence with a valid id, keeping the raw text mounted but hidden', () => {
    render(
      <MarkdownMessageStatic>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessageStatic>,
    )
    expect(screen.getByRole('button', { name: /pasted text/i })).toBeInTheDocument()
    expect(screen.getByText('some long pasted text').closest('.hidden')).not.toBeNull()
  })

  it('renders a plain code block for an excalidraw fence with content that is not valid scene JSON', () => {
    render(<MarkdownMessageStatic>{'```excalidraw:AbC123xy\nnot json\n```'}</MarkdownMessageStatic>)
    expect(screen.getByText('not json')).toBeInTheDocument()
    expect(screen.queryByText(/excalidraw diagram/i)).toBeNull()
  })

  it('renders a diagram preview for a valid excalidraw fence', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(
      <MarkdownMessageStatic>{`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\``}</MarkdownMessageStatic>,
    )
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    // The raw fenced JSON stays mounted (hidden), same as the text-attachment
    // case above — Slate's node<->DOM mapping must never be disturbed.
    expect(screen.getByText(scene).closest('.hidden')).not.toBeNull()
  })

  it("threads a following img node's url through to ExcalidrawPreview as pngRef", () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: 'chats/c1/attachments/diagram.png' })
  })

  it('leaves pngRef undefined when the following node exists but is not an img', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\nSome other paragraph.`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: undefined })
  })

  it('leaves pngRef undefined when there is no following node at all', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(
      <MarkdownMessageStatic>{`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\``}</MarkdownMessageStatic>,
    )

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: undefined })
  })
})
