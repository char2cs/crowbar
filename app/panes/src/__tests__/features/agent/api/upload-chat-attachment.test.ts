import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { ApiError } from '@/lib/errors/api-error'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import * as scopeUrl from '@/lib/workspace-scope-url'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

vi.mock('@/lib/crowbar-bridge', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/crowbar-bridge')>()),
  convertFileSrc: (path: string) => `asset://localhost/${encodeURIComponent(path)}`,
}))

beforeEach(() => {
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

function jsonResponse(body: unknown, status = 201) {
  return new Response(JSON.stringify(body), { status })
}

function parseMultipart(
  body: unknown,
  headers: unknown,
): Map<string, { value: string; filename?: string }> {
  const contentType = (headers as Record<string, string>)['Content-Type']
  const boundary = contentType.split('boundary=')[1]
  const text = new TextDecoder().decode(body as Uint8Array)
  const fields = new Map<string, { value: string; filename?: string }>()
  for (const part of text.split(`--${boundary}`)) {
    const trimmed = part.replace(/^\r\n/, '').replace(/\r\n$/, '')
    if (!trimmed || trimmed === '--') continue
    const [headerBlock, ...rest] = trimmed.split('\r\n\r\n')
    const value = rest.join('\r\n\r\n')
    const nameMatch = /name="([^"]*)"/.exec(headerBlock)
    const filenameMatch = /filename="([^"]*)"/.exec(headerBlock)
    if (!nameMatch) continue
    fields.set(nameMatch[1], { value, filename: filenameMatch?.[1] })
  }
  return fields
}

describe('uploadChatAttachment', () => {
  it('uploads a File as multipart and maps the response envelope', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        data: {
          ref: 'chats/c1/attachments/x-photo.png',
          fileName: 'x-photo.png',
          size: 5,
          contentType: 'image/png',
        },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['hello'], 'photo.png', { type: 'image/png' })
    const result = await uploadChatAttachment('ws1', 'c1', { file }, 'x')

    expect(result).toEqual({
      ref: 'chats/c1/attachments/x-photo.png',
      filename: 'x-photo.png',
      size: 5,
      contentType: 'image/png',
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/c1/attachments')
    expect(init.method).toBe('POST')
    expect(init.body).toBeInstanceOf(Uint8Array)
    const fields = parseMultipart(init.body, init.headers)
    expect(fields.get('id')?.value).toBe('x')
    expect(fields.get('file')).toEqual({ value: 'hello', filename: 'photo.png' })
    vi.unstubAllGlobals()
  })

  it('escapes a quote/backslash in the filename the same way Go multipart.Writer does', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'text/plain' } }),
      )
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['x'], 'weird "name"\\file.txt', { type: 'text/plain' })
    await uploadChatAttachment('ws1', 'c1', { file }, 'x')

    const [, init] = fetchMock.mock.calls[0]
    const contentType = (init.headers as Record<string, string>)['Content-Type']
    const boundary = contentType.split('boundary=')[1]
    const text = new TextDecoder().decode(init.body as Uint8Array)
    expect(text).toContain(`filename="weird \\"name\\"\\\\file.txt"`)
    expect(text.startsWith(`--${boundary}\r\n`)).toBe(true)
    expect(text.endsWith(`--${boundary}--\r\n`)).toBe(true)
    vi.unstubAllGlobals()
  })

  it('strips CR/LF from the filename instead of letting it inject an extra header line', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'text/plain' } }),
      )
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['x'], 'evil\r\nX-Injected: yes.txt', { type: 'text/plain' })
    await uploadChatAttachment('ws1', 'c1', { file }, 'x')

    const [, init] = fetchMock.mock.calls[0]
    const contentType = (init.headers as Record<string, string>)['Content-Type']
    const boundary = contentType.split('boundary=')[1]
    const text = new TextDecoder().decode(init.body as Uint8Array)
    const filePart = text.split(`--${boundary}\r\n`)[2]
    const headerBlock = filePart.split('\r\n\r\n')[0]
    expect(headerBlock.split('\r\n')).toHaveLength(2)
    vi.unstubAllGlobals()
  })

  it('falls back to application/octet-stream when the File carries no type', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'text/plain' } }),
      )
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['x'], 'untyped.bin')
    await uploadChatAttachment('ws1', 'c1', { file }, 'x')

    const [, init] = fetchMock.mock.calls[0]
    const text = new TextDecoder().decode(init.body as Uint8Array)
    expect(text).toContain('Content-Type: application/octet-stream')
    vi.unstubAllGlobals()
  })

  it('uploads a host path as a JSON body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        data: {
          ref: 'chats/c1/attachments/x-dropped.png',
          fileName: 'x-dropped.png',
          size: 9,
          contentType: 'image/png',
        },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await uploadChatAttachment('ws1', 'c1', { path: '/tmp/dropped.png' }, 'x')

    expect(result).toEqual({
      ref: 'chats/c1/attachments/x-dropped.png',
      filename: 'x-dropped.png',
      size: 9,
      contentType: 'image/png',
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/c1/attachments')
    expect(init.method).toBe('POST')
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(JSON.parse(init.body as string)).toEqual({ path: '/tmp/dropped.png', id: 'x' })
    vi.unstubAllGlobals()
  })

  it('mints its own id, matching the shape the fence-tag parser requires, when none is passed', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'image/png' } }),
      )
    vi.stubGlobal('fetch', fetchMock)

    await uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' })

    const [, init] = fetchMock.mock.calls[0]
    const body = JSON.parse(init.body as string)
    expect(body.id).toMatch(/^[A-Za-z0-9_-]{6,}$/)
    vi.unstubAllGlobals()
  })

  it('mints a distinct id on each call when none is passed', async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(
          jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'image/png' } }),
        ),
      )
    vi.stubGlobal('fetch', fetchMock)

    await uploadChatAttachment('ws1', 'c1', { path: '/tmp/a.png' })
    await uploadChatAttachment('ws1', 'c1', { path: '/tmp/b.png' })

    const firstId = JSON.parse(fetchMock.mock.calls[0][1].body as string).id
    const secondId = JSON.parse(fetchMock.mock.calls[1][1].body as string).id
    expect(firstId).not.toBe(secondId)
    vi.unstubAllGlobals()
  })

  it('throws on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('too big', { status: 413 })))
    await expect(uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x')).rejects.toThrow()
    vi.unstubAllGlobals()
  })

  it('rejects a refused upload with the daemon’s own words and code', async () => {
    const refusal = { success: false, error: 'attachment exceeds 20 MB', code: 'too_large' }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(refusal, 413)))

    const upload = uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x')

    await expect(upload).rejects.toBeInstanceOf(ApiError)
    await expect(upload).rejects.toMatchObject({
      status: 413,
      problem: { code: 'too_large', message: 'attachment exceeds 20 MB' },
    })
    vi.unstubAllGlobals()
  })

  it('says a local daemon that is not up yet in the words every request uses', async () => {
    const notReady = new Response('crowbar daemon socket not ready', {
      status: 503,
      headers: { 'x-crowbar-proxy': 'daemon-unavailable' },
    })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(notReady))

    const err = await uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x').catch(
      (e: unknown) => e,
    )
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 503, problem: { code: 'daemon_unavailable' } })
    vi.unstubAllGlobals()
  })
})

describe('uploadChatAttachment on a remote host', () => {
  const remoteBase = 'crowbar://h1/v0/projects/p1/repos/r1/chats'

  beforeEach(() => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    vi.spyOn(scopeUrl, 'repoChatsBaseForWorkspace').mockReturnValue(remoteBase)
    useHostConfigStore.setState({
      hosts: [
        { id: 'h1', name: 'Box', endpoint: 'box:3737', pairedFingerprint: 'ff', clientId: 'c' },
      ],
    })
  })
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    useHostConfigStore.setState({ hosts: [] })
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('posts to that host, not the local daemon', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'image/png' } }),
      )
    vi.stubGlobal('fetch', fetchMock)

    await uploadChatAttachment('ws1', 'c1', { file: new File(['x'], 'x.png') }, 'x')

    expect(fetchMock.mock.calls[0][0]).toBe(`${remoteBase}/c1/attachments`)
  })

  it.each([
    ['could not reach it', { 'x-crowbar-proxy': 'host-unreachable' }, 503, 'host_unreachable'],
    [
      'met another identity',
      { 'x-crowbar-proxy': 'host-identity-changed' },
      502,
      'host_identity_changed',
    ],
    ['was refused by it (401)', {}, 401, 'internal'],
  ])('fails as that host’s when the upload %s', async (_name, headers, status, code) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status, headers })))

    const err = await uploadChatAttachment(
      'ws1',
      'c1',
      { file: new File(['x'], 'x.png') },
      'x',
    ).catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status, problem: { code }, hostId: 'h1' })
  })

  it.each([
    [
      'the daemon’s own words',
      '{"success":false,"error":"attachment too large"}',
      'attachment too large',
    ],
    ['only the status for the proxy’s plain text', 'unknown remote host h1', '502 '],
  ])('fails with %s', async (_name, body, message) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { status: 502 })))

    const err = await uploadChatAttachment(
      'ws1',
      'c1',
      { file: new File(['x'], 'x.png') },
      'x',
    ).catch((e: unknown) => e)

    expect(err).toMatchObject({ message, status: 502, hostId: 'h1' })
  })

  it('fails without sending anything when the host is not configured', async () => {
    useHostConfigStore.setState({ hosts: [] })
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      uploadChatAttachment('ws1', 'c1', { file: new File(['x'], 'x.png') }, 'x'),
    ).rejects.toThrow(/unknown host/)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('uploadChatAttachment from a disk path', () => {
  const UPLOADED = { data: { ref: 'r', fileName: 'pic.png', size: 6, contentType: 'image/png' } }

  beforeEach(() => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    resetHostWorld()
    pairRemoteHost('host-1')
    recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'ws-local' })
    loadProjects(projectOn('p1', 'host-1'), projectOn('p-local', 'local'))
  })
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    resetHostWorld()
    vi.unstubAllGlobals()
  })

  const assetMethods: string[] = []

  function stubFetch(fileResponse: Response, head?: () => Promise<Response>) {
    const uploads: [string, RequestInit][] = []
    assetMethods.length = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.startsWith('asset://')) {
          assetMethods.push(init?.method ?? 'GET')
          if (init?.method === 'HEAD' && head) return head()
          return Promise.resolve(
            init?.method === 'HEAD'
              ? new Response(null, { status: fileResponse.status, headers: fileResponse.headers })
              : fileResponse,
          )
        }
        uploads.push([url, init ?? {}])
        return Promise.resolve(jsonResponse(UPLOADED))
      }),
    )
    return uploads
  }

  const assetResponse = (bytes: string, headers: Record<string, string> = {}) =>
    Object.assign(new Response(null, { headers }), {
      blob: vi.fn(async () => new Blob([bytes], { type: 'image/png' })),
    })

  it('sends a remote chat the file’s bytes as multipart and never its local path', async () => {
    const uploads = stubFetch(assetResponse('pixels'))

    await uploadChatAttachment('ws1', 'c1', { path: '/Users/me/pics/pic.png' }, 'x')

    expect(uploads).toHaveLength(1)
    const [url, init] = uploads[0]
    expect(url).toBe('crowbar://host-1/v0/projects/p1/repos/r1/chats/c1/attachments')
    expect(init.body).toBeInstanceOf(Uint8Array)
    const fields = parseMultipart(init.body, init.headers)
    expect(fields.get('file')).toEqual({ value: 'pixels', filename: 'pic.png' })
    expect(new TextDecoder().decode(init.body as Uint8Array)).not.toContain('/Users/me')
  })

  it('sends nothing when the file cannot be read for a remote chat', async () => {
    const uploads = stubFetch(new Response('forbidden', { status: 403 }))

    await expect(
      uploadChatAttachment('ws1', 'c1', { path: '/Volumes/usb/pic.png' }, 'x'),
    ).rejects.toThrow(/pic\.png/)
    expect(uploads).toEqual([])
  })

  it('keeps handing a local chat the path, whatever hosts are paired', async () => {
    const uploads = stubFetch(new Response('unused'))

    await uploadChatAttachment('ws-local', 'c1', { path: '/tmp/dropped.png' }, 'x')

    const [url, init] = uploads[0]
    expect(url).toBe('/v0/projects/p-local/repos/r1/chats/c1/attachments')
    expect(JSON.parse(init.body as string)).toEqual({ path: '/tmp/dropped.png', id: 'x' })
  })

  it('refuses an oversized file for a remote chat on its HEAD, never fetching its bytes', async () => {
    const answer = assetResponse('x', { 'content-length': String(25 * 1024 * 1024 + 1) })
    const uploads = stubFetch(answer)

    await expect(
      uploadChatAttachment('ws1', 'c1', { path: '/Users/me/big.mov' }, 'x'),
    ).rejects.toThrow(/big\.mov.*25 MB/)
    expect(assetMethods).toEqual(['HEAD'])
    expect(answer.blob).not.toHaveBeenCalled()
    expect(uploads).toEqual([])
  })

  it.each([
    ['answers HEAD with an error', () => Promise.resolve(new Response(null, { status: 405 }))],
    ['rejects HEAD', () => Promise.reject(new Error('unsupported'))],
  ])('still sends the bytes when the asset protocol %s', async (_name, head) => {
    const uploads = stubFetch(assetResponse('pixels'), head)

    await uploadChatAttachment('ws1', 'c1', { path: '/Users/me/pics/pic.png' }, 'x')

    expect(assetMethods).toEqual(['HEAD', 'GET'])
    expect(parseMultipart(uploads[0][1].body, uploads[0][1].headers).get('file')?.value).toBe(
      'pixels',
    )
  })

  it('refuses a file whose size only shows once read, before sending it', async () => {
    const answer = Object.assign(new Response(null), {
      blob: vi.fn(async () => ({ size: 25 * 1024 * 1024 + 1, type: 'video/quicktime' }) as Blob),
    })
    const uploads = stubFetch(answer)

    await expect(
      uploadChatAttachment('ws1', 'c1', { path: '/Users/me/big.mov' }, 'x'),
    ).rejects.toThrow(/big\.mov.*25 MB/)
    expect(assetMethods).toEqual(['HEAD', 'GET'])
    expect(uploads).toEqual([])
  })
})
