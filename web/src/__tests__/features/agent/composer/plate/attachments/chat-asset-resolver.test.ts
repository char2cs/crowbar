import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  chatAttachmentUrl,
  chatMarkdownAssetInfo,
  fetchChatAttachmentDataUrl,
  fetchChatAttachmentMetadata,
  parseChatAttachmentRef,
} from '@/features/agent/composer/plate/attachments/chat-asset-resolver'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

describe('parseChatAttachmentRef', () => {
  it('extracts chatId and filename', () => {
    expect(parseChatAttachmentRef('chats/c1/attachments/pasted-image-1.png')).toEqual({
      chatId: 'c1',
      filename: 'pasted-image-1.png',
    })
  })

  it('extracts a filename containing slashes as everything after attachments/', () => {
    expect(parseChatAttachmentRef('chats/c1/attachments/sub/dir/x.png')).toEqual({
      chatId: 'c1',
      filename: 'sub/dir/x.png',
    })
  })

  it('rejects anything not exactly that shape', () => {
    expect(parseChatAttachmentRef('docs/readme.md')).toBeNull()
    expect(parseChatAttachmentRef('chats/c1/attachments/')).toBeNull()
    expect(parseChatAttachmentRef('https://example.com/x.png')).toBeNull()
    expect(parseChatAttachmentRef('')).toBeNull()
    expect(parseChatAttachmentRef('chats//attachments/x.png')).toBeNull()
    expect(parseChatAttachmentRef('workspace/foo/bar.png')).toBeNull()
  })
})

describe('chatAttachmentUrl', () => {
  it('builds the URL through the same chatBase every chat endpoint uses', () => {
    expect(chatAttachmentUrl('ws1', 'chats/c1/attachments/a b.png')).toBe(
      '/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments/a%20b.png',
    )
  })

  it('returns null for a non-attachment ref', () => {
    expect(chatAttachmentUrl('ws1', 'not-an-attachment')).toBeNull()
  })

  it('returns null (never throws) when the workspace scope is unrecorded', () => {
    expect(() => chatAttachmentUrl('unknown-ws', 'chats/c1/attachments/x.png')).not.toThrow()
    expect(chatAttachmentUrl('unknown-ws', 'chats/c1/attachments/x.png')).toBeNull()
  })
})

describe('fetchChatAttachmentDataUrl', () => {
  // A real `new Response(blob)` is avoided here: this repo's jsdom test
  // environment does not round-trip a jsdom `Blob` through Node/undici's
  // `Response` body handling (it silently stringifies the Blob instead of
  // reading its bytes), which is a test-environment limitation, not a
  // behaviour of the real fetch/Response the browser gives production code.
  // A minimal fetch-result stub with a real `blob()` resolver exercises the
  // same code path (`response.ok`, `await response.blob()`) without hitting
  // that mismatch.
  it('fetches bytes and encodes them as a data: URL', async () => {
    const blob = new Blob(['hello'], { type: 'text/plain' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )
    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toBe(
      'data:text/plain;base64,aGVsbG8=',
    )
    vi.unstubAllGlobals()
  })

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 404 })))
    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toBeNull()
    vi.unstubAllGlobals()
  })

  it('never fetches for a ref that is not a chat attachment', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    expect(await fetchChatAttachmentDataUrl('ws1', 'nope')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('returns null instead of rejecting on a network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))
    await expect(
      fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt'),
    ).resolves.toBeNull()
    vi.unstubAllGlobals()
  })

  it('returns null instead of rejecting when the workspace scope is unrecorded', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    await expect(
      fetchChatAttachmentDataUrl('unknown-ws', 'chats/c1/attachments/x.txt'),
    ).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('forwards the abort signal to fetch', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob(['x'])) })
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()
    await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt', controller.signal)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ signal: controller.signal })
    vi.unstubAllGlobals()
  })

  it('returns null (never rejects) when the FileReader itself reports an error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob(['x'])) }),
    )
    const readSpy = vi.spyOn(FileReader.prototype, 'readAsDataURL').mockImplementation(function (
      this: FileReader,
    ) {
      Object.defineProperty(this, 'error', {
        value: new DOMException('boom'),
        configurable: true,
      })
      this.onerror?.(new ProgressEvent('error') as unknown as ProgressEvent<FileReader>)
    })

    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toBeNull()

    readSpy.mockRestore()
    vi.unstubAllGlobals()
  })

  it('falls back to a generic error when the FileReader reports none', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(new Blob(['x'])) }),
    )
    const readSpy = vi.spyOn(FileReader.prototype, 'readAsDataURL').mockImplementation(function (
      this: FileReader,
    ) {
      this.onerror?.(new ProgressEvent('error') as unknown as ProgressEvent<FileReader>)
    })

    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toBeNull()

    readSpy.mockRestore()
    vi.unstubAllGlobals()
  })
})

describe('fetchChatAttachmentMetadata', () => {
  it('reads size from Content-Length via a HEAD request', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 200, headers: { 'content-length': '1234' } }))
    vi.stubGlobal('fetch', fetchMock)

    expect(await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf')).toEqual({
      filename: 'report.pdf',
      size: 1234,
    })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'HEAD' })
    vi.unstubAllGlobals()
  })

  it('still returns the filename with a null size when Content-Length is absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 200 })))
    expect(await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf')).toEqual({
      filename: 'report.pdf',
      size: null,
    })
    vi.unstubAllGlobals()
  })

  it('returns null for a ref that is not a chat attachment, without fetching', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    expect(await fetchChatAttachmentMetadata('ws1', 'nope')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('returns null instead of rejecting when the workspace scope is unrecorded', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    expect(
      await fetchChatAttachmentMetadata('unknown-ws', 'chats/c1/attachments/report.pdf'),
    ).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('falls back to a null size instead of rejecting on a network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))
    expect(await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf')).toEqual({
      filename: 'report.pdf',
      size: null,
    })
    vi.unstubAllGlobals()
  })

  it('never rejects on a non-ok HEAD response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 404 })))
    await expect(
      fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf'),
    ).resolves.toEqual({ filename: 'report.pdf', size: null })
    vi.unstubAllGlobals()
  })

  // REGRESSION, reported by review: a REAL non-ok response (Gin's own JSON
  // error body, before the backend registered HEAD for this route at all)
  // carries its OWN real Content-Length — an empty-bodied `Response(null,
  // {status: 404})`, the shape the test above uses, never reproduces that,
  // so a missing `response.ok` check passed unnoticed. A non-ok response's
  // Content-Length must never be reported as the file's own size.
  it('ignores Content-Length on a non-ok response, even when one is present', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response('{"error":"not found"}', {
          status: 404,
          headers: { 'content-length': '22' },
        }),
      ),
    )
    await expect(
      fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf'),
    ).resolves.toEqual({ filename: 'report.pdf', size: null })
    vi.unstubAllGlobals()
  })

  it('forwards the abort signal to the HEAD request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()
    await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/x.txt', controller.signal)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'HEAD', signal: controller.signal })
    vi.unstubAllGlobals()
  })
})

describe('chatMarkdownAssetInfo', () => {
  it('produces a resolve() that round-trips through fetchChatAttachmentDataUrl', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )
    const asset = chatMarkdownAssetInfo('ws1')
    expect(await asset.resolve?.('chats/c1/attachments/x.png')).toBe(
      `data:image/png;base64,${btoa('x')}`,
    )
    vi.unstubAllGlobals()
  })

  it('carries wsId and an empty fileDir (unused for chat attachment refs)', () => {
    const asset = chatMarkdownAssetInfo('ws1')
    expect(asset.wsId).toBe('ws1')
    expect(asset.fileDir).toBe('')
  })

  it('resolve() returns null (never rejects) for a non-attachment src', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const asset = chatMarkdownAssetInfo('ws1')
    await expect(asset.resolve?.('not-an-attachment')).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })
})
