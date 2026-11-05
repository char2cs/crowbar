import { useEffect, useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { toast } from '@/features/window/stores/toast-store'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { useChatId } from '@/features/agent/composer/plate/attachments/chat-id-context'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

// Expected URLs are origin-relative; the shell's or .env's VITE_API_URL must not leak in.
beforeEach(() => {
  vi.stubEnv('VITE_API_URL', '')
  resetHostWorld()
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

afterEach(() => {
  vi.unstubAllEnvs()
})

function Probe() {
  const asset = useMarkdownAsset()
  const [resolved, setResolved] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    if (asset && 'resolve' in asset)
      void asset.resolve('chats/c1/attachments/x.png').then(setResolved)
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
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <Probe />
      </ChatMarkdownAssetProvider>,
    )

    await waitFor(() =>
      expect(screen.getByTestId('probe').textContent).toMatch(/^data:image\/png;base64,/),
    )
    expect(fetchMock).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/attachments/x.png',
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
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <CaptureWsId />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen[0]).toMatchObject({ wsId: 'ws1' })
  })

  // A message on another host that links that host's own localhost means that host.
  it('makes the chat’s links its workspace’s: a remote loopback link is not opened here', () => {
    pairRemoteHost('h1', 'Build box')
    loadProjects(projectOn('p1', 'h1'))
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const info = vi.spyOn(toast, 'info').mockReturnValue('')
    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <MarkdownMessageStatic>[preview](http://localhost:3000/)</MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    fireEvent.click(screen.getByRole('link', { name: 'preview' }))

    expect(open).not.toHaveBeenCalled()
    expect(info.mock.calls[0]?.[0]).toMatch(/^This link is on Build box/)
    vi.restoreAllMocks()
  })

  it('supplies the chat id via ChatIdContext', () => {
    const seen: (string | null)[] = []
    function CaptureChatId() {
      seen.push(useChatId())
      return null
    }
    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <CaptureChatId />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen[0]).toBe('c1')
  })

  it('memoizes the value across re-renders with the same wsId', () => {
    const seen: unknown[] = []
    function CaptureRef() {
      seen.push(useMarkdownAsset())
      return null
    }
    const { rerender } = render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    rerender(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
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
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    rerender(
      <ChatMarkdownAssetProvider wsId="ws2" chatId="c1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen).toHaveLength(2)
    expect(seen[0]).not.toBe(seen[1])
  })

  // The asset's loaders (image, excalidraw, html) re-run when its identity
  // changes, which is how an attachment that resolved to nothing while its
  // host was unknown loads once the project list arrives.
  it('hands out a new value when the workspace host resolves, and loads then, not before', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob(['x'])) })
    vi.stubGlobal('fetch', fetchMock)
    pairRemoteHost('host-1')

    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <Probe />
      </ChatMarkdownAssetProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('probe').textContent).toBe('null'))
    expect(fetchMock).not.toHaveBeenCalled()

    act(() => loadProjects(projectOn('p1', 'local')))
    await waitFor(() => expect(screen.getByTestId('probe').textContent).toMatch(/^data:/))
    expect(fetchMock).toHaveBeenCalledTimes(1)
    vi.unstubAllGlobals()
  })
})
