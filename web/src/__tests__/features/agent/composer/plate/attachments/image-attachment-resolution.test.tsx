import { useState } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DndScope } from '@/features/agent/chat/dnd-scope'
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

  // Wrapped in the real `DndScope`: an image is a draggable attachment too
  // (chat-markdown-image-node.tsx).
  it('resolves the same reference in the interactive editor', async () => {
    vi.stubGlobal('fetch', mockImageFetch())

    render(
      <DndScope>
        <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
          <MarkdownMessage>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessage>
        </ChatMarkdownAssetProvider>
      </DndScope>,
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
 * One lead investigated for that second mechanism was
 * `useAttachmentDraggable` (attachment-drag-handle.tsx) building fresh drag
 * options on every render. Exercised below through the real drag stack under
 * `DndScope`: a regression test so a future change to this hook (or to how
 * the composer swaps into the transcript on submit) can't silently
 * reintroduce a render loop without a test noticing.
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
        <DndScope>
          <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
            <ComposerLike n={n} />
          </ChatMarkdownAssetProvider>
          <button onClick={() => setN((v) => v + 1)}>rerender</button>
        </DndScope>
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
  // bubble with the SAME attachment content, sharing one `DndScope`
  // (DndScope wraps both composer and transcript with a single manager).
  it('survives the composer unmounting its attachments as the transcript mounts the same ones, in one commit', () => {
    vi.stubGlobal('fetch', mockImageFetch())

    function Harness() {
      const [sent, setSent] = useState(false)
      const [seed, setSeed] = useState(0)
      return (
        <DndScope>
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
        </DndScope>
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
