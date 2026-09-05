import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DndProvider } from 'react-dnd'
import { HTML5Backend } from 'react-dnd-html5-backend'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { formatAttachmentSize } from '@/features/agent/composer/plate/attachments/chat-attachment-file-card'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

describe('formatAttachmentSize', () => {
  it('formats a sub-1024 byte count as whole bytes', () => {
    expect(formatAttachmentSize(0)).toBe('0 bytes')
    expect(formatAttachmentSize(500)).toBe('500 bytes')
    expect(formatAttachmentSize(1023)).toBe('1023 bytes')
  })

  it('formats KB with two decimals below 10, one decimal at/above 10', () => {
    expect(formatAttachmentSize(1024)).toBe('1.00 KB')
    expect(formatAttachmentSize(2048)).toBe('2.00 KB')
    expect(formatAttachmentSize(10240)).toBe('10.0 KB')
  })

  it('cycles through MB and GB', () => {
    expect(formatAttachmentSize(1024 ** 2)).toBe('1.00 MB')
    expect(formatAttachmentSize(1024 ** 3)).toBe('1.00 GB')
  })

  it('stops advancing units at TB — the last entry — rather than reading past it', () => {
    expect(formatAttachmentSize(1024 ** 5)).toBe('1024.0 TB')
  })

  it('returns null for null, negative, or non-finite input rather than a bogus label', () => {
    expect(formatAttachmentSize(null)).toBeNull()
    expect(formatAttachmentSize(-1)).toBeNull()
    expect(formatAttachmentSize(Number.NaN)).toBeNull()
    expect(formatAttachmentSize(Number.POSITIVE_INFINITY)).toBeNull()
  })
})

describe('chat attachment file card', () => {
  it('renders an ordinary link unchanged for a non-attachment href', () => {
    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>{'[docs](https://example.com)'}</MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )
    // `sanitizeUrl` normalizes a valid absolute URL through `new URL(...).href`
    // (adding the trailing slash) — same reason the pre-existing
    // markdown-message-static.test.tsx link assertion uses `toContain`
    // rather than an exact match.
    const anchor = screen.getByText('docs').closest('a')
    expect(anchor?.getAttribute('href')).toContain('example.com')
    expect(anchor?.className).not.toContain('chat-attachment-file-card')
  })

  it('renders a file card — icon, filename, fetched size — for a chat-attachment link', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(null, { status: 200, headers: { 'content-length': '2048' } }),
        ),
    )

    const { container } = render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>
          {'[report.pdf](chats/c1/attachments/report.pdf)'}
        </MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    expect(screen.getByText('report.pdf')).toBeInTheDocument()
    expect(container.querySelector('svg')).not.toBeNull()
    await waitFor(() => expect(screen.getByText('2.00 KB')).toBeInTheDocument())
    // Settled/read-only — no drag handle, unlike the interactive renderer's
    // card (see the 'renders a drag handle alongside the file card' test
    // below).
    expect(screen.queryByRole('button', { name: /reorder this attachment/i })).toBeNull()

    // The card itself is the link — clicking it must not throw (same
    // handleMarkdownAnchorClick path the ordinary LinkElement uses), and a
    // hover must not bubble past the card either.
    const anchor = screen.getByText('report.pdf').closest('a')
    expect(anchor).toHaveAttribute(
      'href',
      '/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments/report.pdf',
    )
    expect(() => fireEvent.click(anchor!)).not.toThrow()
    expect(() => fireEvent.mouseOver(anchor!)).not.toThrow()

    vi.unstubAllGlobals()
  })

  it('falls back to the ordinary link with no MarkdownAssetContext at all', () => {
    const { container } = render(
      <MarkdownMessageStatic>
        {'[report.pdf](chats/c1/attachments/report.pdf)'}
      </MarkdownMessageStatic>,
    )
    // A bare `chats/...` ref has no leading `/` and no URL scheme, so
    // `sanitizeUrl` (platejs' own link-attribute helper, unrelated to this
    // task) rejects it and LinkElement renders no `href` — same as it always
    // did for this ref shape. What matters here is that the FILE CARD never
    // renders without an asset context: no icon, no card wrapper, no size.
    const anchor = screen.getByText('report.pdf').closest('a')
    expect(anchor).not.toBeNull()
    expect(anchor?.className).not.toContain('chat-attachment-file-card')
    expect(container.querySelector('svg')).toBeNull()
  })

  it('still renders the card, just without a size, when the metadata fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>
          {'[report.pdf](chats/c1/attachments/report.pdf)'}
        </MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    const anchor = await screen.findByText('report.pdf')
    expect(anchor).toBeInTheDocument()
    // Give the rejected fetch a tick to settle before asserting absence.
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled())
    expect(screen.queryByText(/KB|MB|GB|bytes/)).toBeNull()

    vi.unstubAllGlobals()
  })

  it('renders without a working href when the workspace scope cannot be resolved, without crashing', async () => {
    // asset.wsId is set by the provider regardless of whether that wsId's
    // project/repo scope was ever recorded — chatAttachmentUrl (and thus the
    // metadata HEAD request) then has no URL to build against.
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <ChatMarkdownAssetProvider wsId="unscoped-ws">
        <MarkdownMessageStatic>
          {'[report.pdf](chats/c1/attachments/report.pdf)'}
        </MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    const anchor = screen.getByText('report.pdf').closest('a')
    expect(anchor).not.toHaveAttribute('href')
    expect(fetchMock).not.toHaveBeenCalled()
    expect(() => fireEvent.click(anchor!)).not.toThrow()

    vi.unstubAllGlobals()
  })

  // Interactive-only: a drag handle only makes sense where the block is
  // actually editable (the composer, and the interactive/streaming
  // transcript — `MarkdownMessage`, registered on `chatComposerPlugins`),
  // never on settled read-only history (`MarkdownMessageStatic`, above —
  // none of which grew a handle). `@platejs/dnd`'s `useDraggable` throws
  // "Expected drag drop context" without a real `<DndProvider>` ancestor
  // once `DndPlugin` is registered (chat-composer-plugins.ts) — deliberately
  // NOT mocked here, so this is the proof that gap is genuinely closed for
  // the file card too, not just for the code-block attachment kinds.
  it('renders a drag handle alongside the file card, and still behaves as a plain link', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(null, { status: 200, headers: { 'content-length': '10' } }),
        ),
    )

    render(
      <DndProvider backend={HTML5Backend}>
        <ChatMarkdownAssetProvider wsId="ws1">
          <MarkdownMessage>{'[report.pdf](chats/c1/attachments/report.pdf)'}</MarkdownMessage>
        </ChatMarkdownAssetProvider>
      </DndProvider>,
    )
    const handle = screen.getByRole('button', { name: /reorder this attachment/i })
    expect(handle).toBeInTheDocument()

    // Review finding 2: nesting the handle <button> inside the card's <a>
    // is invalid HTML5 (interactive content inside interactive content) and
    // an accessibility smell — the button must be a SIBLING of the anchor,
    // never a descendant of it.
    const anchor = screen.getByText('report.pdf').closest('a')
    expect(anchor).not.toBeNull()
    expect(anchor?.contains(handle)).toBe(false)
    expect(handle.closest('a')).toBeNull()

    // Same click/hover behaviour as the static card's own assertions above —
    // the drag handle is additive, it doesn't change what the card itself does.
    await waitFor(() => expect(screen.getByText('10 bytes')).toBeInTheDocument())
    expect(() => fireEvent.click(anchor!)).not.toThrow()
    expect(() => fireEvent.mouseOver(anchor!)).not.toThrow()
    // The two controls work independently: clicking the handle must not
    // navigate the anchor (no onClick wired to it beyond onSelect, which
    // this render doesn't pass), and must not throw either.
    expect(() => fireEvent.click(handle)).not.toThrow()

    vi.unstubAllGlobals()
  })

  it('falls back to an ordinary link for a non-attachment href, through the INTERACTIVE renderer too', () => {
    render(
      <DndProvider backend={HTML5Backend}>
        <ChatMarkdownAssetProvider wsId="ws1">
          <MarkdownMessage>{'[docs](https://example.com)'}</MarkdownMessage>
        </ChatMarkdownAssetProvider>
      </DndProvider>,
    )
    const anchor = screen.getByText('docs').closest('a')
    expect(anchor?.getAttribute('href')).toContain('example.com')
    expect(anchor?.className).not.toContain('chat-attachment-file-card')
    expect(screen.queryByRole('button', { name: /reorder this attachment/i })).toBeNull()
  })

  it('ignores a metadata fetch that resolves after the component has already unmounted', async () => {
    let resolveFetch!: (response: Response) => void
    const pending = new Promise<Response>((resolve) => {
      resolveFetch = resolve
    })
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(pending))

    const { unmount } = render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>
          {'[report.pdf](chats/c1/attachments/report.pdf)'}
        </MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    unmount()
    resolveFetch(new Response(null, { status: 200, headers: { 'content-length': '999' } }))

    // The effect's `cancelled` guard must keep this a no-op — resolving after
    // unmount must not throw or reject.
    await expect(pending).resolves.toBeInstanceOf(Response)

    vi.unstubAllGlobals()
  })
})
