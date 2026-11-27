import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { TCodeBlockElement } from 'platejs'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { excalidrawSceneFromCodeBlock } from '@/features/agent/composer/plate/attachments/chat-code-block-node'
import * as ExcalidrawPreviewModule from '@/features/agent/composer/plate/attachments/excalidraw-preview'

function renderWithDnd(ui: ReactNode) {
  return render(<DndScope>{ui}</DndScope>)
}

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

vi.mock('@excalidraw/excalidraw', () => ({
  restore: (scene: unknown) => scene,
  exportToSvg: async () => document.createElementNS('http://www.w3.org/2000/svg', 'svg'),
}))

function ComposerDraft({ md }: { md: string }) {
  const editor = usePlateEditor({ plugins: chatComposerPlugins, value: chatMarkdownToValue(md) })
  return (
    <Plate editor={editor} readOnly>
      <PlateContent readOnly />
    </Plate>
  )
}

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
    expect(screen.getByText(scene).closest('.hidden')).not.toBeNull()
  })

  it('renders a diagram preview for a BARE excalidraw fence (no id) with valid scene JSON — agent-authored output', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(<MarkdownMessageStatic>{`\`\`\`excalidraw\n${scene}\n\`\`\``}</MarkdownMessageStatic>)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
  })

  it('renders a plain code block for a bare excalidraw fence with content that is not valid scene JSON', () => {
    render(<MarkdownMessageStatic>{'```excalidraw\nnot json\n```'}</MarkdownMessageStatic>)
    expect(screen.getByText('not json')).toBeInTheDocument()
    expect(screen.queryByText(/excalidraw diagram/i)).toBeNull()
  })

  it('renders a drag handle for a text-attachment pill', () => {
    renderWithDnd(
      <MarkdownMessage>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessage>,
    )
    expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()
  })

  it('renders no drag handle for a plain, non-attachment code block — even under a real DndScope', () => {
    renderWithDnd(<MarkdownMessage>{'```go\nfunc main() {}\n```'}</MarkdownMessage>)
    expect(screen.queryByRole('button', { name: /reorder this attachment/i })).toBeNull()
  })

  it('renders a delete button alongside the drag handle, which removes the attachment on click', async () => {
    renderWithDnd(
      <MarkdownMessage>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessage>,
    )
    expect(screen.getByRole('button', { name: /remove this attachment/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))

    await waitFor(() => expect(screen.queryByRole('button', { name: /pasted text/i })).toBeNull())
  })

  it('renders no delete button for a plain, non-attachment code block', () => {
    renderWithDnd(<MarkdownMessage>{'```go\nfunc main() {}\n```'}</MarkdownMessage>)
    expect(screen.queryByRole('button', { name: /remove this attachment/i })).toBeNull()
  })

  it('removing an excalidraw attachment also removes its own persisted-PNG sibling, not just the fence', async () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    renderWithDnd(
      <ComposerDraft
        md={`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/x-diagram.png)`}
      />,
    )
    expect(document.querySelectorAll('.hidden')).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))

    await waitFor(() => expect(screen.queryByText(/excalidraw diagram/i)).toBeNull())
    expect(document.querySelectorAll('.hidden')).toHaveLength(0)
  })

  it('wraps the drag handle in its own non-editable island, not just relying on an ancestor', () => {
    renderWithDnd(
      <MarkdownMessage>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessage>,
    )
    const handle = screen.getByRole('button', { name: /reorder this attachment/i })
    const island = handle.closest('[contenteditable="false"]')
    expect(island).not.toBeNull()
    expect(island?.classList.contains('slate-editor')).toBe(false)
  })

  it("threads a following img node's url through to ExcalidrawPreview as pngRef", () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: 'chats/c1/attachments/diagram.png' })
  })

  it('does not also render the excalidraw PNG sibling as a plain markdown image', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    expect(document.querySelectorAll('img')).toHaveLength(0)
  })

  it('still renders an ordinary image normally when it is not an excalidraw PNG sibling', () => {
    render(
      <MarkdownMessageStatic>{'![a photo](https://example.com/photo.png)'}</MarkdownMessageStatic>,
    )
    expect(document.querySelectorAll('img')).toHaveLength(1)
  })

  it('renders the image normally when it follows an excalidraw fence whose content is not a valid scene', () => {
    const md =
      '```excalidraw:AbC123xy\nnot json\n```\n\n![diagram](chats/c1/attachments/diagram.png)'
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    expect(document.querySelectorAll('img')).toHaveLength(1)
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

  it('threads pngRef correctly through the INTERACTIVE renderer too, not just its static derivative', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    renderWithDnd(<MarkdownMessage>{md}</MarkdownMessage>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: 'chats/c1/attachments/diagram.png' })
  })
})

function codeBlockElement(lang: string, lines: string[]): TCodeBlockElement {
  return {
    type: 'code_block',
    lang,
    children: lines.map((text) => ({ type: 'code_line', children: [{ text }] })),
  } as unknown as TCodeBlockElement
}

describe('excalidrawSceneFromCodeBlock identity stability', () => {
  it('returns the SAME scene reference for two different node objects with identical source text', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const a = codeBlockElement('excalidraw:AbC123xy', [scene])
    const b = codeBlockElement('excalidraw:AbC123xy', [scene])
    expect(a).not.toBe(b)
    expect(excalidrawSceneFromCodeBlock(a)).toBe(excalidrawSceneFromCodeBlock(b))
  })
})
