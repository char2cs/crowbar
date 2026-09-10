import { createRef, type ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PlateEditor } from 'platejs/react'
import type { TCodeBlockElement } from 'platejs'
import { DndProvider } from 'react-dnd'
import { HTML5Backend } from 'react-dnd-html5-backend'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import {
  excalidrawSceneFromCodeBlock,
  findFollowingImageRef,
} from '@/features/agent/composer/plate/attachments/chat-code-block-node'
import * as ExcalidrawPreviewModule from '@/features/agent/composer/plate/attachments/excalidraw-preview'

/**
 * The real (unmocked) `<DndProvider>` this app's production code puts above
 * every Plate tree that can render an attachment node (`AgentChatView`'s own
 * `DndScope`, see agent-chat-view.tsx) — `@platejs/dnd`'s `useDraggable`
 * throws "Expected drag drop context" without one, once `DndPlugin` is
 * registered (chat-composer-plugins.ts). Standalone tests that mount
 * `MarkdownMessage` directly, bypassing `AgentChatView` entirely, have to
 * supply this same ancestor themselves — deliberately NOT mocking
 * `@platejs/dnd` here (unlike attachment-drag-handle.test.tsx's own unit
 * tests) is what proves the gap is genuinely closed, not just papered over.
 */
function renderWithDnd(ui: ReactNode) {
  return render(<DndProvider backend={HTML5Backend}>{ui}</DndProvider>)
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

  it('renders no drag handle for a plain, non-attachment code block — even under a real DndProvider', () => {
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
  // BEFORE react-dnd's own native `dragstart` ever fires, regardless of the
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

  // REGRESSION, reported live: dragging still did nothing even after the
  // non-editable-island fix above, because Tauri's own OS-file-drop
  // interception (`dragDropEnabled`, on by default — see tauri-file-drop.ts)
  // swallows a real native drag gesture at the OS/webview boundary before it
  // ever becomes a DOM `dragstart` event, independent of the contenteditable
  // fix. `DndScope` (dnd-scope.tsx) now wires the app's ONE `<DndProvider>` to
  // `TouchBackend` instead of `HTML5Backend` specifically so dragging never
  // depends on that native API at all — this test renders through the REAL
  // `DndScope` (not this file's own `HTML5Backend`-based `renderWithDnd`
  // helper, which every other test here uses only to satisfy `useDraggable`'s
  // context requirement) and drives a drag with plain `mousedown`/`mousemove`,
  // exactly what a real WebKit user gesture produces — proving the fix
  // without needing a native `dragstart` event jsdom cannot simulate anyway.
  it('starts a real drag from a plain mousedown+mousemove, matching what Tauri actually delivers', () => {
    // jsdom implements neither `elementsFromPoint` (used by `TouchBackend`
    // to resolve hover targets under the pointer) nor real layout — a real
    // WebKit browser has both, so this stub exists only to satisfy jsdom,
    // not to change what's under test (whether a drag STARTS at all).
    const elementsFromPoint = vi.fn().mockReturnValue([])
    document.elementsFromPoint = elementsFromPoint

    render(
      <DndScope>
        <MarkdownMessage>
          {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
        </MarkdownMessage>
      </DndScope>,
    )
    const handle = screen.getByRole('button', { name: /reorder this attachment/i })
    const block = handle.closest('.slate-code_block')
    expect(block?.className).not.toContain('opacity-50')

    fireEvent.mouseDown(handle, { clientX: 0, clientY: 0 })
    fireEvent.mouseMove(document, { clientX: 0, clientY: 20 })

    expect(block?.className).toContain('opacity-50')

    // `DndScope`'s manager is a MODULE-LEVEL singleton (dnd-scope.tsx),
    // shared across every test in this file — an unterminated drag here
    // leaks `monitor.isDragging()` into whichever DnD test runs next. A real
    // gesture always ends in a `mouseup`; this one has to too.
    fireEvent.mouseUp(document, { clientX: 0, clientY: 20 })
  })

  // REGRESSION, reported live: "I can't move attachment between paragraphs"
  // — dragging an attachment worked, but only ever swapped it with ANOTHER
  // attachment: nothing made a plain paragraph a valid `useDropNode` target
  // at all, and different attachment kinds used their own Slate node type as
  // the dnd type (a code-block's `'code_block'` vs. a file card's `'p'`),
  // so even two attachments of different kinds couldn't drop on each other.
  // `ChatParagraphElement` (chat-paragraph-node.tsx) now registers every
  // plain paragraph as a drop target via `useAttachmentDropTarget`, and
  // `ATTACHMENT_DND_TYPE` unifies the dnd type across every attachment kind.
  //
  // A DEEPER cause surfaced chasing this one down: `@platejs/dnd`'s hover
  // resolution (`getHoverDirection`) bails whenever the hovered candidate's
  // `.id` matches the dragged item's — which it always did, because nothing
  // in `chatComposerPlugins` ever assigned an `.id` at all (confirmed
  // directly: no plugin under any key did this — an incorrect assumption
  // from earlier in this feature's build, never actually verified). Fixed by
  // registering `NodeIdPlugin` (chat-composer-plugins.ts). Plate's own
  // `withNodeId` only assigns ids through a real INSERT transform, not to
  // nodes already present in an editor's initial `value` — this is why the
  // test below builds its document via `ChatMarkdownEditor`'s real
  // `insertAttachmentMarkdown` handle (the same path `insertAttachmentMarkdown
  // Into`/the composer's own paste and attach flows use), not a canned
  // `MarkdownMessage` initial value the way every other test in this file
  // does; a canned value never goes through the insert transform, so it
  // never gets ids either, no differently from before this fix.
  //
  // `TouchBackend`'s drop-target hover detection (`connectDropTarget`,
  // react-dnd-touch-backend) is a SEPARATE code path from the drag-start
  // detection the test above exercises — it listens on `document.body`
  // (not `root`/`document` directly) and calls `document.elementFromPoint`
  // (singular) to decide whether the current position is over a given
  // target's own node, so both need their own jsdom stand-ins here; a real
  // WebKit browser implements both natively.
  //
  // The target paragraph has to be one the attachment ISN'T already sitting
  // next to: `getDropPath` (@platejs/dnd) deliberately no-ops a "move" that
  // would land the dragged item exactly where it already is (dragging onto
  // the bottom of the paragraph directly above it, say) — confirmed by
  // instrumenting it directly, this is intentional, not a bug. A second,
  // later paragraph gives the drag somewhere real to go.
  it('registers a plain paragraph as a drop target, so dragging an attachment over one shows the drop-line there too', async () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    render(
      <DndScope>
        <ChatMarkdownEditor
          ref={ref}
          initialValue="first paragraph"
          placeholder=""
          ariaLabel="Message the agent"
          onChange={vi.fn()}
          onKeyDown={vi.fn()}
        />
      </DndScope>,
    )
    await act(async () => {
      ref.current?.insertAttachmentMarkdown(
        '```text-attachment:AbC123xy\nsome long pasted text\n```',
      )
      ref.current?.insertAttachmentMarkdown('second paragraph')
    })

    const handle = await screen.findByRole('button', { name: /reorder this attachment/i })
    const paragraph = screen.getByText('second paragraph').closest('.slate-p')
    expect(paragraph).not.toBeNull()

    // Both singular AND plural stand-ins are needed: `connectDropTarget`'s
    // own per-target hover check (which populates `dragOverTargetIds`) uses
    // `elementFromPoint`, while `handleTopMove`'s separate ordering pass
    // (which the ACTUAL hover dispatch is filtered through) uses
    // `elementsFromPoint` — leaving the plural one returning `[]` (as the
    // earlier, drag-START-only test above does, which never needs it) means
    // that filter always empties out, so `hover()` never fires for ANY
    // target, no matter what the singular stand-in says.
    document.elementFromPoint = vi.fn().mockReturnValue(paragraph)
    document.elementsFromPoint = vi.fn().mockReturnValue([paragraph])

    fireEvent.mouseDown(handle, { clientX: 0, clientY: 0 })
    // The FIRST move is what flips `isDragging` true (react-dnd-touch-
    // backend's own `handleTopMove`, on `document`) — but `connectDropTarget`'s
    // separate hover check (on `document.body`, fires earlier in the bubble
    // order for the SAME event) bails immediately while `isDragging()` is
    // still false, so it never even calls `elementFromPoint` on this first
    // move (confirmed by instrumenting it directly). A real drag always has
    // more than one `mousemove` before the pointer settles on a target, so a
    // second one here matches what actually reaches a drop target's hover
    // detection, not just what starts the drag.
    fireEvent.mouseMove(paragraph!, { clientX: 0, clientY: 20 })
    fireEvent.mouseMove(paragraph!, { clientX: 0, clientY: 25 })

    expect(paragraph!.querySelector('.-top-px, .-bottom-px')).not.toBeNull()

    // Same cleanup reasoning as the drag-start test above — `DndScope`'s
    // manager is shared across this whole file's tests.
    fireEvent.mouseUp(paragraph!, { clientX: 0, clientY: 25 })
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
    // ChatCodeBlockElement), so this needs the same real `<DndProvider>`
    // ancestor production wires up at AgentChatView.
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
