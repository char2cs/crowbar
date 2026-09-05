import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

function jsonResponse(body: unknown, status = 201) {
  return new Response(JSON.stringify(body), { status })
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
    expect(init.body).toBeInstanceOf(FormData)
    const form = init.body as FormData
    expect(form.get('id')).toBe('x')
    const uploaded = form.get('file') as File
    expect(uploaded.name).toBe('photo.png')
    expect(await uploaded.text()).toBe('hello')
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
