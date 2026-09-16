import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

function jsonResponse(body: unknown, status = 201) {
  return new Response(JSON.stringify(body), { status })
}

/** Decodes the hand-rolled multipart body `uploadChatAttachment` now sends
 *  (see upload-chat-attachment.ts's own note on why it isn't `FormData`) back
 *  into a `Map<name, {value, filename?}>`, using the boundary off the
 *  matching request header — the same thing a real multipart parser reads. */
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
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments')
    expect(init.method).toBe('POST')
    // Not a FormData: a WKWebView/Tauri custom-protocol body-loss bug drops any
    // Blob-backed fetch body (a FormData holding a File, or a bare Blob) before
    // it reaches the daemon — see upload-chat-attachment.ts's own note. The
    // body must be a plain Uint8Array so it always survives that proxy.
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

  // REGRESSION, reported by review: backslash/quote were the only characters
  // escaped, so a filename carrying a literal CR/LF split the hand-built
  // Content-Disposition line in two, injecting an extra header line into the
  // multipart body this client itself constructs. Self-request-only (this
  // is the CLIENT'S own outgoing request), but a real gap regardless.
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
    // The file part's own header block (Content-Disposition, Content-Type) —
    // an unstripped CR/LF in the filename would split Content-Disposition
    // into two lines, so this would be 3 lines instead of 2.
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
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments')
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

  it('throws an error carrying the status and body text on a non-ok response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(() => Promise.resolve(new Response('too big', { status: 413 }))),
    )
    await expect(uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x')).rejects.toThrow(
      /413/,
    )
    await expect(uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x')).rejects.toThrow(
      /too big/,
    )
    vi.unstubAllGlobals()
  })
})
