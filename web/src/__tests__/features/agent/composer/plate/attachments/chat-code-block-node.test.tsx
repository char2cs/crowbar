import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PlateEditor } from 'platejs/react'
import type { TCodeBlockElement } from 'platejs'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { DndScope } from '@/features/agent/chat/dnd-scope'
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

  // REGRESSION: an agent has no way to know the composer's own `:{id}`
  // suffix convention — it never goes through excalidrawMarkdown, it just
  // writes an ```excalidraw fence in its reply text like any other code
  // block. A bare tag used to always fall through to plain code, so a
  // diagram the AGENT produced never rendered as one.
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

  // REGRESSION, reported live: there was no way to select and remove an
  // attachment at all — dragging could reorder one but nothing could delete
  // one.
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

  // REGRESSION, reported live: deleting an excalidraw attachment removed the
  // fence but left its own persisted-PNG sibling behind — invisible (still
  // hidden, since `isExcalidrawPngSibling` never re-evaluates once its
  // scene's preceding fence is gone) but very much still in the document, so
  // the "deleted" diagram silently reappeared in the message once sent. The
  // two nodes are one logical attachment (`excalidraw-takeover.tsx` inserts
  // them as an adjacent pair); deleting either must remove both.
  it('removing an excalidraw attachment also removes its own persisted-PNG sibling, not just the fence', async () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    renderWithDnd(
      <MarkdownMessage>
        {`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/x-diagram.png)`}
      </MarkdownMessage>,
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

  // REGRESSION, reported live: dragging an attachment did nothing at all —
  // root-caused via live DOM inspection to the handle sitting directly
  // inside the Slate editor's own `contenteditable="true"` region with no
  // non-editable island around it. WebKit (Tauri's WKWebView) arbitrates a
  // real mousedown+move inside editable content as a text-selection gesture
  // BEFORE the drag can start, regardless of the
  // button's `draggable="true"` — confirmed live (`-webkit-user-drag:
  // element` and `draggable="true"` were both already correct; the only gap
  // was the missing `contenteditable="false"` boundary).
  //
  // `MarkdownMessage` (this file's harness) renders `readOnly`, so its own
  // Slate root already carries `contenteditable="false"` regardless of this
  // fix — asserting merely "some ancestor is non-editable" would pass
  // trivially either way. The real contract is a wrapper CLOSER than the
  // editor root, since the actual composer (which this stands in for) is
  // genuinely editable and has no such root-level escape hatch.
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

  // REGRESSION, reported live: the excalidraw PNG sibling — consumed by
  // ExcalidrawPreview as `pngRef` — was ALSO rendering as an ordinary markdown
  // image right underneath it, showing the same diagram twice in one message.
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

// REGRESSION: React error #185 ("Maximum update depth exceeded"), reported
// live as a pane crash on submitting a message with an attachment.
// `excalidrawSceneFromCodeBlock` used to re-parse the fence's own JSON on
// every call, handing back a BRAND NEW scene object each time — including
// when the ONLY thing that changed was the `TCodeBlockElement` object
// identity itself (exactly what happens when Slate recreates a node for a
// reason that has nothing to do with its own text: `NodeIdPlugin`
// reassigning ids, an unrelated edit elsewhere in the document). That fresh
// object flowed straight into `ExcalidrawPreview`'s own `useEffect` deps
// array (`scene`, excalidraw-preview.tsx), tearing down and restarting its
// async export-to-svg pipeline on every one of those unrelated re-renders.
describe('excalidrawSceneFromCodeBlock identity stability', () => {
  it('returns the SAME scene reference for two different node objects with identical source text', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const a = codeBlockElement('excalidraw:AbC123xy', [scene])
    const b = codeBlockElement('excalidraw:AbC123xy', [scene])
    expect(a).not.toBe(b)
    expect(excalidrawSceneFromCodeBlock(a)).toBe(excalidrawSceneFromCodeBlock(b))
  })
})

/**
 * `findFollowingImageRef`'s own unit coverage for `editor.api.findPath`
 * returning `undefined` — the guard exists for `findPath`'s own `Path |
 * undefined` return contract (an element no longer in the tree), not
 * anything a mounted document can trigger, so it's tested directly rather
 * than through a full render.
 *
 * This function deliberately calls `editor.api.findPath(props.element)`
 * fresh rather than trusting `props.path`: in the interactive renderer,
 * `props.path` is sourced from Plate's `useNodePath`, memoized on
 * `[editor.api, node]` — its own JSDoc says it will NOT recompute when a
 * change elsewhere shifts this node's path without replacing the node
 * object. A black-box regression test for that exact scenario was attempted
 * (mount a real interactive editor via `chatComposerPlugins`, insert a
 * sibling before this block, assert the sibling-image lookup still finds the
 * right node) and dropped: it passed identically whether `findFollowingImageRef`
 * used `props.path` or `findPath`, because slate-react's own `MemoizedElement`
 * wrapper (`prev.element === next.element`, confirmed by reading
 * `node_modules/slate-react/dist/index.js`) skips re-rendering this node at
 * all unless its OWN `element` reference changes — which invalidates
 * `useNodePath`'s memo too, so by the time this component's render body ever
 * runs again, the path is never actually stale in practice. That is an
 * internal implementation detail of a dependency two layers down, not a
 * documented contract, so `findPath` is used anyway — it costs one extra
 * (rare-path) tree-walk and matches Plate's own stated `useNodePath` caveat
 * rather than depending on slate-react's current memoization strategy never
 * changing.
 */
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
