import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))

import { ApiError, apiFetch } from '@/lib/api'
import {
  HOST_DOCUMENT_TRANSPORT_CODE,
  readDocument,
  readDocumentText,
  workspaceRef,
  writeDocument,
  type DocumentRef,
} from '@/lib/api/documents'

const fetchMock = apiFetch as ReturnType<typeof vi.fn>
const WS_REF: DocumentRef = { authority: 'workspace', workspaceId: 'w1', path: 'src/a.ts' }

beforeEach(() => {
  fetchMock.mockReset()
})

describe('readDocument', () => {
  it('reads a workspace document by workspace and relative path', async () => {
    fetchMock.mockResolvedValue({ ...WS_REF, content: 'export {}\n' })

    const read = await readDocument(WS_REF)

    expect(fetchMock).toHaveBeenCalledWith('/v0/documents/content?workspace=w1&path=src%2Fa.ts')
    expect(read).toEqual({ ref: WS_REF, text: 'export {}\n' })
  })

  it('reads an absolute path and returns the document the daemon resolved', async () => {
    fetchMock.mockResolvedValue({ authority: 'host', path: '/private/tmp/x.md', content: '# x' })

    const read = await readDocument({ absolutePath: '/tmp/x.md' })

    expect(fetchMock).toHaveBeenCalledWith('/v0/documents/content?path=%2Ftmp%2Fx.md')
    expect(read).toEqual({ ref: { authority: 'host', path: '/private/tmp/x.md' }, text: '# x' })
  })

  it('decodes a base64 payload byte-faithfully', async () => {
    fetchMock.mockResolvedValue({ ...WS_REF, content: btoa('\x00\xff\xe9'), encoding: 'base64' })

    const read = await readDocument(WS_REF)

    expect(read.text).toBe('\x00\xff\xe9')
  })

  it('rejects a host read off the local socket with the transport code', async () => {
    fetchMock.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    await expect(readDocument({ absolutePath: '/etc/hosts' })).rejects.toMatchObject({
      status: 403,
      code: 'host_document_requires_local_transport',
    })
  })
})

describe('readDocumentText', () => {
  it('answers the text, or null when the read fails', async () => {
    fetchMock
      .mockResolvedValueOnce({ ...WS_REF, content: 'x' })
      .mockRejectedValueOnce(new Error('gone'))

    expect(await readDocumentText(workspaceRef('w1', 'src/a.ts'))).toBe('x')
    expect(await readDocumentText(workspaceRef('w1', 'src/a.ts'))).toBeNull()
  })
})

describe('writeDocument', () => {
  it('writes base64 bytes when asked', async () => {
    const ref: DocumentRef = { authority: 'workspace', workspaceId: 'w1', path: 'a.bin' }
    fetchMock.mockResolvedValue(ref)

    const written = await writeDocument(ref, 'AA==', { encoding: 'base64' })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/documents/content')
    expect(init.method).toBe('PUT')
    expect(JSON.parse(init.body as string)).toEqual({
      workspaceId: 'w1',
      path: 'a.bin',
      content: 'AA==',
      encoding: 'base64',
    })
    expect(written).toEqual(ref)
  })

  it('writes a host document by its absolute path alone', async () => {
    const ref: DocumentRef = { authority: 'host', path: '/tmp/x.md' }
    fetchMock.mockResolvedValue(ref)

    await writeDocument(ref, 'text')

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ path: '/tmp/x.md', content: 'text' })
  })
})
