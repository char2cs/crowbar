import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createFolder, placeFolder, deleteFolder } from '@/lib/api/sidebar-placement'

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ success: true, data }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// Task 34: the dedicated `/repos/:repoId/folders` resource was deleted from the
// backend (11b72c72) — folders are Chat rows now, served only via
// `.../chats/folders`, and its Create/Patch/Delete respond with `{folder,
// shifted}` / `{shifted}` (dto.AgentChatDTO rows, title-named), not the old bare
// `{id}` / void these three functions used to assume.
describe('createFolder', () => {
  it('POSTs .../chats/folders and reshapes {folder, shifted} into FolderDTOs', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        folder: { id: 'f9', parentId: '', title: 'New folder', order: 0 },
        shifted: [{ id: 'f1', parentId: '', title: 'F1', order: 1 }],
      }),
    )
    const { folder, shifted } = await createFolder('p1', 'r1', 'New folder', '')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/folders')
    expect(url).not.toContain('/repos/r1/folders')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'New folder', parentId: '' })
    expect(folder).toEqual({
      id: 'f9',
      repoId: 'r1',
      projectId: 'p1',
      parentId: '',
      name: 'New folder',
      order: 0,
    })
    expect(shifted).toEqual([
      { id: 'f1', repoId: 'r1', projectId: 'p1', parentId: '', name: 'F1', order: 1 },
    ])
  })

  it('defaults shifted to [] when the backend omits the field', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ folder: { id: 'f9', parentId: '', title: 'X', order: 0 } }),
    )
    const { shifted } = await createFolder('p1', 'r1', 'X', '')
    expect(shifted).toEqual([])
  })
})

describe('placeFolder', () => {
  it('PATCHes .../chats/folders/:folderId and reshapes the response', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        folder: { id: 'f1', parentId: 'f2', title: 'Renamed', order: 3 },
        shifted: [],
      }),
    )
    const { folder } = await placeFolder('p1', 'r1', 'f1', { parentId: 'f2', order: 3 })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/folders/f1')
    expect(init).toMatchObject({
      method: 'PATCH',
      body: JSON.stringify({ parentId: 'f2', order: 3 }),
    })
    expect(folder.name).toBe('Renamed')
  })
})

describe('deleteFolder', () => {
  it('DELETEs .../chats/folders/:folderId and returns the promoted-children shift', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ shifted: [{ id: 'f2', parentId: '', title: 'F2', order: 0 }] }),
    )
    const shifted = await deleteFolder('p1', 'r1', 'f1')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/folders/f1')
    expect(init).toMatchObject({ method: 'DELETE' })
    expect(shifted).toEqual([
      { id: 'f2', repoId: 'r1', projectId: 'p1', parentId: '', name: 'F2', order: 0 },
    ])
  })

  it('defaults to [] on a null body', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: null }), { status: 200 }),
    )
    expect(await deleteFolder('p1', 'r1', 'f1')).toEqual([])
  })
})
