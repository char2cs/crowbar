import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
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
      <ChatMarkdownAssetProvider wsId="ws1">
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

  it('resolves the same reference in the interactive editor', async () => {
    vi.stubGlobal('fetch', mockImageFetch())

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessage>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessage>
      </ChatMarkdownAssetProvider>,
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
