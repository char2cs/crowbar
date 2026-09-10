import { useState } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DndProvider } from 'react-dnd'
import { HTML5Backend } from 'react-dnd-html5-backend'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })

// A real `new Response(blob)` is avoided here: this repo's jsdom test
// environment does not round-trip a jsdom `Blob` through Node/undici's
// `Response` body handling (it silently stringifies the Blob instead of
// reading its bytes) — see chat-asset-resolver.test.ts for the same note. A
// minimal fetch-result stub with a real `blob()` resolver exercises the same
// `response.ok` / `await response.blob()` path fetchChatAttachmentDataUrl
// actually calls, without hitting that mismatch.
function mockImageFetch() {
  const blob = new Blob(['x'], { type: 'image/png' })
  return vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) })
}

describe('chat image attachment resolution', () => {
  it('resolves ![alt](ref) to real fetched bytes in the static transcript render', async () => {
    const fetchMock = mockImageFetch()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <MarkdownMessageStatic>
          {'![a diagram](chats/c1/attachments/shot.png)'}
        </MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: 'a diagram' })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/chats/c1/attachments/shot.png'),
      expect.anything(),
    )
    vi.unstubAllGlobals()
  })

  // Wrapped in a real `<DndProvider>`: an image is now a draggable attachment
  // too (chat-markdown-image-node.tsx), same as a text-attachment or file
  // card already were — `useAttachmentDraggable` throws "Expected drag drop
  // context" once it actually renders without one.
  it('resolves the same reference in the interactive editor', async () => {
    vi.stubGlobal('fetch', mockImageFetch())

    render(
      <DndProvider backend={HTML5Backend}>
        <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
          <MarkdownMessage>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessage>
        </ChatMarkdownAssetProvider>
      </DndProvider>,
    )

    const img = await screen.findByRole('img', { name: 'a diagram' })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    vi.unstubAllGlobals()
  })

  it('leaves the raw src alone with no MarkdownAssetContext (pre-existing behaviour, unchanged)', () => {
    render(
      <MarkdownMessageStatic>
        {'![a diagram](chats/c1/attachments/shot.png)'}
      </MarkdownMessageStatic>,
    )
    expect(screen.getByRole('img', { name: 'a diagram' }).getAttribute('src')).toBe(
      'chats/c1/attachments/shot.png',
    )
  })
})

const TWO_ATTACHMENTS = [
  {
    type: 'p',
    children: [{ type: 'a', url: 'chats/c1/attachments/a.pdf', children: [{ text: 'a.pdf' }] }],
  },
  {
    type: 'img',
    url: 'chats/c1/attachments/b.png',
    caption: [{ text: 'b.png' }],
    children: [{ text: '' }],
  },
]

/**
 * REGRESSION, investigated live: React error #185 ("Maximum update depth
 * exceeded"), reported on submitting a message with an attachment, turned
 * out to have (at least) two independent mechanisms. The first — an
 * Excalidraw scene re-parsed into a fresh object every render, torn down and
 * restarted by `ExcalidrawPreview`'s own effect — is fixed and covered by
 * `excalidrawSceneFromCodeBlock identity stability` (chat-code-block-node.
 * test.tsx). The user then confirmed live that the SAME crash also happens
 * with a PLAIN photo/file attachment and no Excalidraw involved at all — a
 * second, independent mechanism.
 *
 * The strongest lead investigated for that second mechanism was
 * `useAttachmentDraggable` (attachment-drag-handle.tsx): it builds a fresh
 * `drag: { end }` options object on every render and hands it to `@platejs/
 * dnd`'s `useDraggable`. Read all the way down through `@platejs/dnd` and
 * `react-dnd`'s own source (node_modules, this version) rather than assumed:
 *
 * - The DRAG-source half (`useDragNode`) calls `react-dnd`'s `useDrag` with
 *   an explicit `[editor, elementId]` deps array — `useOptionalFactory`
 *   (react-dnd) memoizes on THAT, not on the spec object's own identity, so
 *   a fresh `end` closure per render never even reaches a re-registration
 *   once mounted.
 * - The DROP-target half (`useDropNode` → `useDrop`) is called with NO deps,
 *   which `useOptionalFactory` treats as "recompute every render" BY DESIGN
 *   — but `useDropTarget` (react-dnd) only re-assigns `dropTarget.spec` in a
 *   plain `useEffect`, never tearing down/re-running the actual
 *   `registerTarget` layout effect, whose own deps (`manager`, `monitor`,
 *   the memoized `DropTargetImpl` instance, a stringified `accept` list)
 *   stay referentially stable regardless of the spec object's identity.
 *
 * Confirmed empirically below, through the REAL (unmocked) `@platejs/dnd` +
 * `react-dnd` stack — not the mocked-boundary unit tests in attachment-
 * drag-handle.test.tsx, which verify composition, not library internals.
 * This is NOT a fix: it is evidence the lead, however plausible-sounding,
 * does not reproduce the crash — recorded as a regression test so a future
 * change to this hook (or to how the composer swaps into the transcript on
 * submit) can't silently reintroduce it without a test noticing.
 */
describe('REGRESSION investigation: React error #185 with a plain (non-Excalidraw) attachment', () => {
  function ComposerLike({ n }: { n: number }) {
    const editor = usePlateEditor({
      plugins: chatComposerPlugins,
      value: [...TWO_ATTACHMENTS, { type: 'p', children: [{ text: `draft ${n}` }] }],
    })
    return (
      <Plate editor={editor}>
        <PlateContent />
      </Plate>
    )
  }

  // Simulates an attachment node re-rendering for a reason that has nothing
  // to do with it — a sibling edit, streaming text elsewhere in the same
  // document, NodeIdPlugin churn — many times in a row, forcing a brand new
  // `drag: { end }` closure into `useDraggable` on every single one.
  //
  // Wrapped in `ChatMarkdownAssetProvider` so the link actually renders as
  // `ChatAttachmentFileCard` (which calls `useAttachmentDraggable`), not the
  // plain-link fallback `ChatLinkElement` uses with no asset context.
  it('survives 60 unrelated re-renders of a file-card + image attachment pair with no runaway update-depth error', () => {
    vi.stubGlobal('fetch', mockImageFetch())

    function Harness() {
      const [n, setN] = useState(0)
      return (
        <DndProvider backend={HTML5Backend}>
          <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
            <ComposerLike n={n} />
          </ChatMarkdownAssetProvider>
          <button onClick={() => setN((v) => v + 1)}>rerender</button>
        </DndProvider>
      )
    }

    const { getByText } = render(<Harness />)
    for (let i = 0; i < 60; i++) {
      act(() => {
        getByText('rerender').click()
      })
    }
    expect(screen.getByRole('link', { name: /a\.pdf/ })).toBeInTheDocument()
    vi.unstubAllGlobals()
  })

  // Simulates the actual submit gesture: `enqueueDraft` (agent-chat-view.tsx)
  // clears the composer via a `key` change on `ComposerField` (agent-
  // composer.tsx), unmounting every attachment it held — including their
  // `useAttachmentDraggable`/`useAttachmentDropTarget` registrations — in
  // the SAME commit the transcript mounts a new (interactive, streaming)
  // bubble with the SAME attachment content, sharing one `<DndProvider>`
  // (DndScope wraps both composer and transcript with a single manager).
  it('survives the composer unmounting its attachments as the transcript mounts the same ones, in one commit', () => {
    vi.stubGlobal('fetch', mockImageFetch())

    function Harness() {
      const [sent, setSent] = useState(false)
      const [seed, setSeed] = useState(0)
      return (
        <DndProvider backend={HTML5Backend}>
          <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
            {sent && (
              <MarkdownMessage>
                {'[a.pdf](chats/c1/attachments/a.pdf)\n\n![b.png](chats/c1/attachments/b.png)'}
              </MarkdownMessage>
            )}
            {!sent && <ComposerLike key={seed} n={seed} />}
          </ChatMarkdownAssetProvider>
          <button
            onClick={() => {
              setSent(true)
              setSeed((s) => s + 1)
            }}
          >
            submit
          </button>
        </DndProvider>
      )
    }

    const { getByText } = render(<Harness />)
    act(() => {
      getByText('submit').click()
    })
    expect(screen.getByRole('link', { name: /a\.pdf/ })).toBeInTheDocument()
    vi.unstubAllGlobals()
  })
})
