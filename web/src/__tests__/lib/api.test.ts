import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { chatDTOFromWire, workspaceDTOFromChat, type RepoChatWireDTO } from '@/lib/api/wire'
import { fetchFolders, fetchHomeWorkspace, fetchRepoChats } from '@/lib/api'
import type { ChatWorktreeDTO } from '@/lib/types'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('fetchHomeWorkspace', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches from the correct endpoint and returns JSON', async () => {
    const dto = { id: 'ws-home', projectId: 'p1', kind: 'home' }
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, data: dto }), { status: 200 }),
    )
    const result = await fetchHomeWorkspace('p1')
    expect(result).toEqual(dto)
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toContain('/v0/projects/p1/home')
  })

  it('throws on non-2xx response', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ success: false, error: 'not found' }), { status: 404 }),
    )
    await expect(fetchHomeWorkspace('p1')).rejects.toThrow()
  })
})

describe('fetchFolders', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // Task 34: the dedicated `/folders` resource was deleted from the backend
  // (11b72c72) — folders are Chat rows now, served only via .../chats/folders,
  // whose wire DTO names its text field `title`, not `name`. Both the route and
  // the reshape are load-bearing: hitting the dead route 404s, and reading `name`
  // off the real DTO renders every folder blank.
  it('GETs .../chats/folders and reshapes the title-named wire DTO into a FolderDTO', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: 'f1', parentId: 'f0', title: 'Spikes', order: 2 }],
        }),
        { status: 200 },
      ),
    )
    const folders = await fetchFolders('p1', 'r1')
    expect(vi.mocked(fetch).mock.calls[0][0]).toContain('/v0/projects/p1/repos/r1/chats/folders')
    expect(vi.mocked(fetch).mock.calls[0][0]).not.toContain('/repos/r1/folders')
    expect(folders).toEqual([
      { id: 'f1', repoId: 'r1', projectId: 'p1', parentId: 'f0', name: 'Spikes', order: 2 },
    ])
  })

  it('returns [] when the backend responds with no body', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
    expect(await fetchFolders('p1', 'r1')).toEqual([])
  })
})

describe('fetchRepoChats', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The CONVERSATION half of the same repo-scoped `.../chats` mount the folder
  // half above reads from — design spec §3.1's `chat` row, which the sidebar
  // tree needs and never had. Route and reshape are both load-bearing for the
  // same reasons: the wire DTO names its text `title`, and there is no
  // repo-scoped chat route other than this one.
  it('GETs .../repos/:r/chats and reshapes the wire DTO into a ChatDTO', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: 'c1', workspaceId: 'ws1', parentId: 'f0', title: 'Fix parser', order: 2 }],
        }),
        { status: 200 },
      ),
    )
    const chats = await fetchRepoChats('p1', 'r1')
    expect(vi.mocked(fetch).mock.calls[0][0]).toContain('/v0/projects/p1/repos/r1/chats')
    // Never the FOLDER half of the same mount — that returns folder-typed rows.
    expect(vi.mocked(fetch).mock.calls[0][0]).not.toContain('/chats/folders')
    expect(chats).toEqual([
      {
        id: 'c1',
        repoId: 'r1',
        projectId: 'p1',
        type: undefined,
        workspaceId: 'ws1',
        // No `worktree` on the wire row, so this row holds no worktree of its
        // own to own — see the dedicated `ownsWorktree` block below.
        ownsWorktree: false,
        parentId: 'f0',
        title: 'Fix parser',
        order: 2,
      },
    ])
  })

  // A BUBBLE (spec §3.1): owns no workspace, filed nowhere. Both empties are
  // real answers and must survive the reshape rather than being dropped, since
  // the tree reads them to decide where the row hangs.
  it('carries an empty workspaceId and parentId through as the bubble they mean', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: 'c1', workspaceId: '', parentId: '', title: '', order: 0 }],
        }),
        { status: 200 },
      ),
    )
    expect(await fetchRepoChats('p1', 'r1')).toEqual([
      {
        id: 'c1',
        repoId: 'r1',
        projectId: 'p1',
        type: undefined,
        workspaceId: '',
        // Holds no worktree, so it owns none — the third of the three shapes
        // `ownsWorktree` distinguishes (owner / thread carrying its parent's /
        // bubble).
        ownsWorktree: false,
        parentId: '',
        title: '',
        order: 0,
      },
    ])
  })

  // The one fact that tells a locked-branch or repo-home row apart from an
  // ordinary conversation inside the SAME repo-scoped list. `dto.AgentChatDTO`
  // never omits it ("" is not a real ChatType), and dropping it here left the
  // sidebar unable to tell which chat row IS a workspace's row.
  it('carries the row’s own type through the reshape', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [
            { id: 'b1', workspaceId: 'ws1', parentId: '', title: '', order: 0, type: 'branch' },
            { id: 'c1', workspaceId: 'ws1', parentId: '', title: 'Talk', order: 1, type: 'chat' },
          ],
        }),
        { status: 200 },
      ),
    )
    expect((await fetchRepoChats('p1', 'r1')).map((c) => c.type)).toEqual(['branch', 'chat'])
  })

  it('stamps THIS call’s repo/project on every row — the wire carries neither', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: 'c1', workspaceId: 'ws1', parentId: '', title: 'a', order: 0 }],
        }),
        { status: 200 },
      ),
    )
    const [chat] = await fetchRepoChats('p9', 'r9')
    expect(chat.repoId).toBe('r9')
    expect(chat.projectId).toBe('p9')
  })

  it('returns [] when the backend responds with no body', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
    expect(await fetchRepoChats('p1', 'r1')).toEqual([])
  })
})

/**
 * OWNERSHIP TRAVELS WITH THE CHAT.
 *
 * `worktree` rides EVERY row holding a workspace — a thread carries its
 * parent's — so `owningChatId` is the only thing that picks the one row that IS
 * that workspace. `workspaceDTOFromChat` has always spent that predicate to
 * derive a separate `WorkspaceDTO`; keeping it on the chat as well is what lets
 * the sidebar decide a row's KIND without waiting on a record that arrives on a
 * different stream (see `ChatDTO.ownsWorktree`).
 */
describe('chatDTOFromWire — ownsWorktree', () => {
  const worktree: ChatWorktreeDTO = {
    provisioning: 'provisioned',
    branch: 'feature/one',
    owningChatId: 'c-owner',
    working: false,
    added: 0,
    deleted: 0,
    mergeStrategy: '',
    canMergeLocally: false,
    mergeConflicts: false,
  }

  it('marks the row that owns the worktree', () => {
    const dto = chatDTOFromWire(
      { id: 'c-owner', workspaceId: 'ws-1', parentId: '', title: '', order: 0, worktree },
      'p1',
      'r1',
    )

    expect(dto.ownsWorktree).toBe(true)
  })

  it('does NOT mark a thread that merely carries its parent’s worktree', () => {
    const dto = chatDTOFromWire(
      { id: 'c-thread', workspaceId: 'ws-1', parentId: 'c-owner', title: '', order: 0, worktree },
      'p1',
      'r1',
    )

    expect(dto.ownsWorktree).toBe(false)
    // Same workspace, same `worktree` object — `workspaceId` alone could never
    // have told these two rows apart.
    expect(dto.workspaceId).toBe('ws-1')
  })

  it('does not mark a bubble that holds no worktree at all', () => {
    const dto = chatDTOFromWire(
      { id: 'c-bubble', workspaceId: '', parentId: '', title: '', order: 0 },
      'p1',
      'r1',
    )

    expect(dto.ownsWorktree).toBe(false)
  })

  it('agrees with workspaceDTOFromChat on every row', () => {
    const rows: RepoChatWireDTO[] = [
      { id: 'c-owner', workspaceId: 'ws-1', parentId: '', title: '', order: 0, worktree },
      { id: 'c-thread', workspaceId: 'ws-1', parentId: 'c-owner', title: '', order: 1, worktree },
      { id: 'c-bubble', workspaceId: '', parentId: '', title: '', order: 2 },
    ]

    for (const row of rows) {
      const ownsPerChat = chatDTOFromWire(row, 'p1', 'r1').ownsWorktree
      const ownsPerWorkspace = workspaceDTOFromChat(row, 'p1', 'r1') !== null
      expect(ownsPerChat).toBe(ownsPerWorkspace)
    }
  })
})
