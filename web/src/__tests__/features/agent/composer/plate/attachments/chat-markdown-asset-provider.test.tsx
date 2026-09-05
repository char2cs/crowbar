import { useEffect, useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

function Probe() {
  const asset = useMarkdownAsset()
  const [resolved, setResolved] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    void asset?.resolve?.('chats/c1/attachments/x.png').then(setResolved)
  }, [asset])
  return <span data-testid="probe">{resolved === undefined ? 'pending' : String(resolved)}</span>
}

describe('ChatMarkdownAssetProvider', () => {
  // A real `new Response(blob)` is avoided here: this repo's jsdom test
  // environment does not round-trip a jsdom `Blob` through Node/undici's
  // `Response` body handling (it silently stringifies the Blob instead of
  // reading its bytes) — see chat-asset-resolver.test.ts for the same note.
  // A minimal fetch-result stub with a real `blob()` resolver exercises the
  // same code path end to end without hitting that mismatch.
  it('provides an asset whose resolve() fetches through the chat-attachment endpoint', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <Probe />
      </ChatMarkdownAssetProvider>,
    )

    await waitFor(() =>
      expect(screen.getByTestId('probe').textContent).toMatch(/^data:image\/png;base64,/),
    )
    expect(fetchMock).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments/x.png',
      expect.anything(),
    )
    vi.unstubAllGlobals()
  })

  it('supplies wsId on the provided asset info', () => {
    const seen: ReturnType<typeof useMarkdownAsset>[] = []
    function CaptureWsId() {
      seen.push(useMarkdownAsset())
      return null
    }
    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureWsId />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen[0]?.wsId).toBe('ws1')
  })

  it('memoizes the value across re-renders with the same wsId', () => {
    const seen: unknown[] = []
    function CaptureRef() {
      seen.push(useMarkdownAsset())
      return null
    }
    const { rerender } = render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    rerender(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen).toHaveLength(2)
    expect(seen[0]).toBe(seen[1])
  })

  it('produces a new value when wsId changes', () => {
    const seen: unknown[] = []
    function CaptureRef() {
      seen.push(useMarkdownAsset())
      return null
    }
    const { rerender } = render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    rerender(
      <ChatMarkdownAssetProvider wsId="ws2">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen).toHaveLength(2)
    expect(seen[0]).not.toBe(seen[1])
  })
})
