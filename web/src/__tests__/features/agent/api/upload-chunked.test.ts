import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { CHUNK_BYTES } from '@/features/agent/api/upload-chunked'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

const BASE = '/v0/projects/p1/repos/r1/chats/c1/attachments'

function envelope(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status })
}

/** An in-memory daemon speaking the chunked-upload routes, holding the bytes per upload id. */
function fakeDaemon(fail?: (offset: number) => boolean) {
  const held = new Map<string, number>()
  const calls: string[] = []
  const handler = vi.fn(async (url: string, init: RequestInit) => {
    const [path, query] = url.slice(BASE.length).split('?')
    calls.push(`${init.method} ${path}${query ? `?${query}` : ''}`)
    if (path === '/uploads') {
      const { id } = JSON.parse(init.body as string) as { id: string }
      return envelope({ received: held.get(id) ?? 0 })
    }
    const id = path.split('/')[2]
    if (init.method === 'DELETE') {
      held.delete(id)
      return new Response(null, { status: 204 })
    }
    if (init.method === 'PUT') {
      const offset = Number(new URLSearchParams(query).get('offset'))
      if (fail?.(offset)) return Promise.reject(new TypeError('Network request failed'))
      held.set(id, offset + (init.body as Uint8Array).length)
      return envelope({ received: held.get(id) })
    }
    return envelope(
      { ref: `r/${id}`, fileName: `${id}-a.bin`, size: held.get(id), contentType: 't' },
      201,
    )
  })
  return { handler, held, calls }
}

const bigFile = (chunks: number, extra = 5) =>
  new File([new Uint8Array(chunks * CHUNK_BYTES + extra)], 'a.bin')

beforeEach(() => recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' }))
afterEach(() => vi.unstubAllGlobals())

describe('uploadChatAttachment in chunks', () => {
  it('sends a file larger than a chunk piece by piece, progress rising to the whole size', async () => {
    const daemon = fakeDaemon()
    vi.stubGlobal('fetch', daemon.handler)
    const file = bigFile(2)
    const seen: number[] = []

    const result = await uploadChatAttachment('ws1', 'c1', { file }, 'up1', {
      onProgress: (loaded, total) => {
        expect(total).toBe(file.size)
        seen.push(loaded)
      },
    })

    expect(seen).toEqual([0, CHUNK_BYTES, 2 * CHUNK_BYTES, file.size])
    expect(daemon.calls).toEqual([
      'POST /uploads',
      'PUT /uploads/up1?offset=0',
      `PUT /uploads/up1?offset=${CHUNK_BYTES}`,
      `PUT /uploads/up1?offset=${2 * CHUNK_BYTES}`,
      'POST /uploads/up1/complete',
    ])
    expect(result).toEqual({
      ref: 'r/up1',
      filename: 'up1-a.bin',
      size: file.size,
      contentType: 't',
    })
  })

  it('sends a file of one chunk or less in a single request, as before', async () => {
    const daemon = fakeDaemon()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => envelope({ ref: 'r', fileName: 'f', size: 1, contentType: 't' }, 201)),
    )
    await uploadChatAttachment(
      'ws1',
      'c1',
      { file: new File([new Uint8Array(CHUNK_BYTES)], 'a') },
      'x',
    )
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(daemon.calls).toEqual([])
  })

  it('resumes from the bytes the daemon already holds when the same id is sent again', async () => {
    let dropAt: number | undefined = CHUNK_BYTES
    const daemon = fakeDaemon((offset) => offset === dropAt)
    vi.stubGlobal('fetch', daemon.handler)
    const file = bigFile(2)

    await expect(uploadChatAttachment('ws1', 'c1', { file }, 'up1')).rejects.toBeDefined()
    expect(daemon.held.get('up1')).toBe(CHUNK_BYTES)

    dropAt = undefined
    daemon.calls.length = 0
    const seen: number[] = []
    await uploadChatAttachment('ws1', 'c1', { file }, 'up1', { onProgress: (l) => seen.push(l) })

    expect(seen[0]).toBe(CHUNK_BYTES)
    expect(daemon.calls).not.toContain('PUT /uploads/up1?offset=0')
    expect(seen.at(-1)).toBe(file.size)
  })

  it('aborting stops sending and deletes what the daemon staged', async () => {
    const daemon = fakeDaemon()
    const controller = new AbortController()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init: RequestInit) => {
        if (init.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
        return daemon.handler(url, init)
      }),
    )

    const upload = uploadChatAttachment('ws1', 'c1', { file: bigFile(3) }, 'up1', {
      signal: controller.signal,
      onProgress: (loaded) => loaded >= CHUNK_BYTES && controller.abort(),
    })

    await expect(upload).rejects.toBeDefined()
    expect(daemon.calls.at(-1)).toBe('DELETE /uploads/up1')
    expect(daemon.held.has('up1')).toBe(false)
    expect(daemon.calls.filter((c) => c.startsWith('PUT'))).toHaveLength(1)
  })
})
