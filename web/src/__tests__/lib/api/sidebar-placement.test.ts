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

// 2026-09-09 sidebar-placement-unification, workspace-placement fix: a
// locked branch's own row is addressed by the WORKSPACE itself now —
// `PATCH .../workspaces/:wsId/placement` — never by the chat that owns its
// worktree. No owningChatId needs recording for this call at all any more;
// workspaceBase resolves straight off the project/repo scope every
// workspace already gets recorded with (recordWorkspaceScope, sidebar
// store), locked branch or not.
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
