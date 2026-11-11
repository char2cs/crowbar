import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PlateEditor } from 'platejs/react'
import type { TCodeBlockElement } from 'platejs'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  excalidrawSceneFromCodeBlock,
  findFollowingImageRef,
} from '@/features/agent/composer/plate/attachments/chat-code-block-node'
import * as ExcalidrawPreviewModule from '@/features/agent/composer/plate/attachments/excalidraw-preview'

/** The app renders attachments under `AgentChatView`'s `DndScope`. */
function renderWithDnd(ui: ReactNode) {
  return render(<DndScope>{ui}</DndScope>)
}

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

/** The composer's own interactive editor: the transcript renders diagrams instead. */
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
    // The raw fenced JSON stays mounted (hidden), same as the text-attachment
    // case above — Slate's node<->DOM mapping must never be disturbed.
    expect(screen.getByText(scene).closest('.hidden')).not.toBeNull()
  })

  // An agent writes a bare ```excalidraw fence (no composer `:{id}` suffix), and that must still
  // render as a diagram, not plain code.
  it('renders a diagram preview for a BARE excalidraw fence (no id) with valid scene JSON — agent-authored output', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(<MarkdownMessageStatic>{`\`\`\`excalidraw\n${scene}\n\`\`\``}</MarkdownMessageStatic>)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
  })

  // The false-positive guard still holds for the bare-tag path: content
  // validation, not the id suffix, is what's doing the work here.
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

  // Each attachment has a delete button beside its drag handle.
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

  // The fence and its persisted-PNG sibling are one logical attachment (inserted as an adjacent
  // pair); deleting the fence must take the sibling too, or the hidden PNG ships with the message.
  it('removing an excalidraw attachment also removes its own persisted-PNG sibling, not just the fence', async () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    renderWithDnd(
      <ComposerDraft
        md={`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/x-diagram.png)`}
      />,
    )
    // Two mounted-but-hidden raw nodes going in: the fence's own raw source,
    // and the PNG sibling's void placeholder (`ChatMarkdownImageElement`'s
    // `isExcalidrawPngSibling` branch) — both `className="hidden"`.
    expect(document.querySelectorAll('.hidden')).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))

    await waitFor(() => expect(screen.queryByText(/excalidraw diagram/i)).toBeNull())
    // Both gone — not just the fence's own, leaving the sibling's orphaned.
    expect(document.querySelectorAll('.hidden')).toHaveLength(0)
  })

  // WKWebView treats mousedown+move inside editable content as text selection before a drag can
  // start, so the handle needs its own contenteditable="false" island. The harness's Slate root is
  // already read-only, hence asserting a wrapper CLOSER than the editor root.
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

  // The PNG sibling feeds ExcalidrawPreview's `pngRef`; it must not also render as a plain image.
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

  // Every test above renders through MarkdownMessageStatic. This is the one
  // exercising `MarkdownMessage`'s INTERACTIVE editor — the other surface
  // `ChatCodeBlockElement` is registered on (chatComposerPlugins, not just
  // its static derivative) — so the sibling-lookup logic is proven to run
  // there too, not only under static rendering. It does not, and cannot by
  // itself, prove immunity to `useNodePath`'s memoized-staleness caveat (see
  // `findFollowingImageRef`'s own doc comment and this file's bottom
  // describe block for why that specific scenario has no reliable black-box
  // reproduction); it establishes that the interactive path renders
  // correctly at all, which no other test here did before.
  it('threads pngRef correctly through the INTERACTIVE renderer too, not just its static derivative', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    // An excalidraw fence resolves to a preview, which — under the
    // interactive renderer — is wrapped in the draggable primitive (see
    // ChatCodeBlockElement), so this renders under the same `DndScope`
    // production wires up at AgentChatView.
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

// The scene must keep its identity when only the TCodeBlockElement object changes (NodeIdPlugin,
// unrelated edits): a fresh scene re-runs ExcalidrawPreview's export effect on every render, which
// loops into React error #185 (maximum update depth).
describe('excalidrawSceneFromCodeBlock identity stability', () => {
  it('returns the SAME scene reference for two different node objects with identical source text', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const a = codeBlockElement('excalidraw:AbC123xy', [scene])
    const b = codeBlockElement('excalidraw:AbC123xy', [scene])
    expect(a).not.toBe(b)
    expect(excalidrawSceneFromCodeBlock(a)).toBe(excalidrawSceneFromCodeBlock(b))
  })
})

/** Covers findPath returning `undefined` (its `Path | undefined` contract), which no mounted
 *  document triggers. findPath is used over `props.path` because Plate's useNodePath memo may not
 *  recompute when a sibling shifts this node, per its own JSDoc. */
describe('findFollowingImageRef', () => {
  it('returns undefined, without looking up a sibling, when the element has no path', () => {
    const nodeSpy = vi.fn()
    const fakeEditor = {
      api: { findPath: () => undefined, node: nodeSpy },
    } as unknown as PlateEditor
    const result = findFollowingImageRef({
      editor: fakeEditor,
      element: { type: 'code_block', children: [] },
    } as unknown as Parameters<typeof findFollowingImageRef>[0])

    expect(result).toBeUndefined()
    expect(nodeSpy).not.toHaveBeenCalled()
  })
})
