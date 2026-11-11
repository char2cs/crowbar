import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  createFolder,
  placeFolder,
  deleteFolder,
  placeWorkspace,
} from '@/lib/api/sidebar-placement'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

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

// Folders are served via `.../chats/folders`; Create/Patch/Delete answer `{folder, shifted}` /
// `{shifted}` as title-named rows, reshaped here into FolderDTOs.
describe('createFolder', () => {
  it('POSTs .../chats/folders and reshapes {folder, shifted} into FolderDTOs', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        folder: { id: 'f9', type: 'folder', parentId: '', title: 'New folder', order: 0 },
        shifted: [{ id: 'f1', type: 'folder', parentId: '', title: 'F1', order: 1 }],
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
      jsonResponse({ folder: { id: 'f9', type: 'folder', parentId: '', title: 'X', order: 0 } }),
    )
    const { shifted } = await createFolder('p1', 'r1', 'X', '')
    expect(shifted).toEqual([])
  })
})

describe('placeFolder', () => {
  it('PATCHes .../chats/folders/:folderId and reshapes the response', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        folder: { id: 'f1', type: 'folder', parentId: 'f2', title: 'Renamed', order: 3 },
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

  // A locked branch shares the folder's level; the daemon reports it among
  // `shifted` as the branch row it draws as (its workspace id). It is never a
  // folder — routed to `shiftedRows`, so no ghost folder is ever applied.
  it('splits a shifted locked-branch sibling out of the folder rows', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        folder: { id: 'f1', type: 'folder', parentId: '', title: 'F1', order: 0 },
        shifted: [
          { id: 'f2', type: 'folder', parentId: '', title: 'F2', order: 1 },
          { id: 'ws-locked', type: 'branch', parentId: '', title: '', order: 2 },
        ],
      }),
    )
    const { shifted, shiftedRows } = await placeFolder('p1', 'r1', 'f1', { order: 0 })
    expect(shifted.map((f) => f.id)).toEqual(['f2'])
    expect(shiftedRows).toEqual([{ id: 'ws-locked', parentId: '', order: 2 }])
  })
})

describe('deleteFolder', () => {
  it('DELETEs .../chats/folders/:folderId and returns the promoted-children shift', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        shifted: [{ id: 'f2', type: 'folder', parentId: '', title: 'F2', order: 0 }],
      }),
    )
    const shifted = await deleteFolder('p1', 'r1', 'f1')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/folders/f1')
    expect(init).toMatchObject({ method: 'DELETE' })
    expect(shifted).toEqual([
      { id: 'f2', repoId: 'r1', projectId: 'p1', parentId: '', name: 'F2', order: 0 },
    ])
  })

  // A promoted child can be a chat: it is never answered as a folder, or it would land as a ghost
  // folder entity under the chat's id.
  it('answers only the promoted folders, never a promoted chat', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        shifted: [
          { id: 'f2', type: 'folder', parentId: '', title: 'F2', order: 0 },
          { id: 'c1', type: 'chat', parentId: '', title: 'A chat', order: 1 },
        ],
      }),
    )
    expect((await deleteFolder('p1', 'r1', 'f1')).map((f) => f.id)).toEqual(['f2'])
  })

  it('defaults to [] on a null body', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: null }), { status: 200 }),
    )
    expect(await deleteFolder('p1', 'r1', 'f1')).toEqual([])
  })
})

// A locked branch's row is addressed by the workspace (`PATCH .../workspaces/:wsId/placement`),
// never its owning chat, so no owningChatId need be recorded — only the project/repo scope.
describe('placeWorkspace', () => {
  beforeEach(() => {
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1' })
  })

  it('PATCHes the workspace-addressed placement route, never a chat one', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ workspace: {}, shifted: [] }))

    await placeWorkspace('ws-1', { folderId: 'f-rev', order: 2 })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces/ws-1/placement')
    expect(url).not.toContain('/chats/')
    expect(init.method).toBe('PATCH')
  })

  // The moved row's decided placement and every shifted sibling are the only confirmation a branch
  // drag gets (no workspace frame carries a placement).
  it('answers the moved row and the shifted siblings, never void', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({
        workspace: { id: 'ws-1', parentId: 'f-rev', order: 2 },
        shifted: [{ id: 'f1', type: 'folder', parentId: 'f-rev', title: 'F1', order: 3 }],
      }),
    )

    expect(await placeWorkspace('ws-1', { folderId: 'f-rev', order: 2 })).toEqual({
      workspace: { id: 'ws-1', parentId: 'f-rev', order: 2 },
      shifted: [{ id: 'f1', parentId: 'f-rev', order: 3 }],
    })
  })

  it('defaults shifted to [] when the backend omits the field', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ workspace: { id: 'ws-1', parentId: '', order: 0 } }))

    expect((await placeWorkspace('ws-1', { order: 0 })).shifted).toEqual([])
  })

  // A folder and a locked branch's own row share ONE sibling space within
  // the branch's own repo, so the route names the destination `parentId`.
  it('sends the folder as parentId, the field the placement route reads', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ workspace: {}, shifted: [] }))

    await placeWorkspace('ws-1', { folderId: 'f-rev', order: 2 })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ parentId: 'f-rev', order: 2 })
  })

  // Both fields are "leave it as it is" when omitted. Sending an explicit
  // null/undefined would re-root the row.
  it('omits a field the caller did not set rather than sending an empty one', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ workspace: {}, shifted: [] }))

    await placeWorkspace('ws-1', { order: 0 })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ order: 0 })
  })

  // The repo ROOT is a real destination, spelled '' — distinct from "unset".
  it('sends the repo root as an empty parentId, not as an omission', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ workspace: {}, shifted: [] }))

    await placeWorkspace('ws-1', { folderId: '', order: 1 })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ parentId: '', order: 1 })
  })
})
