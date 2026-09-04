import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  fetchRepos,
  fetchWorkspace,
  fetchWorkspaces,
  apiFetch,
  workspaceDTOFromChat,
} from '@/lib/api'
import type { RepoChatWireDTO } from '@/lib/api'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import type { ChatWorktreeDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

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

describe('fetchRepos', () => {
  it('GETs /v0/projects/:projectId/repos and returns the RepoDTO list', async () => {
    const repos: RepoDTO[] = [
      {
        id: 'r1',
        projectId: 'p1',
        name: 'crowbar',
        path: '/tmp/crowbar',
        defaultBranch: 'main',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        avatarUrl: '',
        avatarEmoji: '',
      },
    ]
    fetchMock.mockResolvedValue(jsonResponse(repos))
    const result = await fetchRepos('p1')
    const [url] = fetchMock.mock.calls[0] as [string]
    expect(url).toBe('/v0/projects/p1/repos')
    expect(result).toEqual(repos)
  })
})

// A worktree is HELD BY A CHAT now, so there is no workspace list to read: the
// git half rides each chat row as a nested `worktree`, and the WorkspaceDTOs are
// derived from the chat list. Several rows can carry ONE worktree (a thread
// carries its parent's `workspaceId`), and every one of them gets the object —
// so the owning-row rule is what keeps the result one DTO per worktree.
const worktree = (over: Partial<ChatWorktreeDTO> = {}): ChatWorktreeDTO => ({
  branch: 'feature/x',
  status: 'new',
  working: false,
  added: 0,
  deleted: 0,
  mergeStrategy: 'squash',
  canMergeLocally: true,
  mergeConflicts: false,
  owningChatId: 'c1',
  ...over,
})

const chatRow = (over: Partial<RepoChatWireDTO> = {}): RepoChatWireDTO => ({
  id: 'c1',
  workspaceId: 'w1',
  parentId: '',
  title: 'alpha',
  order: 0,
  type: 'branch',
  ...over,
})

describe('workspaceDTOFromChat', () => {
  it('maps every field of the owning row', () => {
    const row = chatRow({
      worktree: worktree({
        branch: 'feature/x',
        status: 'pr-open',
        lastError: 'boom',
        working: true,
        isDefault: true,
        added: 3,
        deleted: 1,
        mergeStrategy: 'squash',
        canMergeLocally: true,
        mergeConflicts: true,
        parentBranch: 'main',
        prUrl: 'https://example.test/pr/1',
        prTitle: 'Add x',
        prTargetBranch: 'main',
        localPath: '/x/y',
        heldByPath: '/held/here',
        forkPointSha: 'abc123',
        parentId: 'ws-parent',
      }),
    })

    expect(workspaceDTOFromChat(row, 'p1', 'r1')).toEqual({
      id: 'w1',
      repoId: 'r1',
      projectId: 'p1',
      branch: 'feature/x',
      parentId: 'ws-parent',
      forkPointSha: 'abc123',
      status: 'pr-open',
      working: true,
      lastError: 'boom',
      isDefault: true,
      added: 3,
      deleted: 1,
      mergeStrategy: 'squash',
      canMergeLocally: true,
      mergeConflicts: true,
      parentBranch: 'main',
      prUrl: 'https://example.test/pr/1',
      prTitle: 'Add x',
      prTargetBranch: 'main',
      localPath: '/x/y',
      heldByPath: '/held/here',
      owningChatId: 'c1',
    } satisfies WorkspaceDTO)
  })

  it('grounds every omitted field rather than leaving it undefined', () => {
    // Every optional is `omitempty` on the wire, so an absent one is the empty
    // value — and it has to arrive as one, because the sidebar merges a live
    // frame over a seeded row field by field and an undefined never clears.
    expect(workspaceDTOFromChat(chatRow({ worktree: worktree() }), 'p1', 'r1')).toMatchObject({
      status: 'new',
      lastError: '',
      isDefault: false,
      parentId: '',
      forkPointSha: '',
      parentBranch: '',
      prUrl: '',
      prTitle: '',
      prTargetBranch: '',
      localPath: '',
      heldByPath: '',
    })
  })

  it('returns null for a bubble row, which holds no worktree at all', () => {
    expect(
      workspaceDTOFromChat(chatRow({ id: 'c9', workspaceId: '', type: 'chat' }), 'p1', 'r1'),
    ).toBeNull()
  })

  it('returns null for a NON-owning row sharing the same worktree', () => {
    // A thread carries its parent's workspaceId AND its parent's worktree
    // object, owningChatId and all. Only the row the id names is that
    // worktree's row.
    const thread = chatRow({ id: 'c2', parentId: 'c1', type: 'chat', worktree: worktree() })
    expect(workspaceDTOFromChat(thread, 'p1', 'r1')).toBeNull()
  })

  it('returns null when the row names no workspace to key the DTO by', () => {
    expect(
      workspaceDTOFromChat(chatRow({ workspaceId: '', worktree: worktree() }), 'p1', 'r1'),
    ).toBeNull()
  })
})

describe('fetchWorkspaces', () => {
  it('GETs the repo CHAT list and derives the WorkspaceDTOs from it', async () => {
    fetchMock.mockResolvedValue(jsonResponse([chatRow({ worktree: worktree() })]))
    const result = await fetchWorkspaces('p1', 'r1')
    const [url] = fetchMock.mock.calls[0] as [string]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats')
    expect(result).toEqual([
      {
        id: 'w1',
        repoId: 'r1',
        projectId: 'p1',
        branch: 'feature/x',
        parentId: '',
        forkPointSha: '',
        status: 'new',
        working: false,
        lastError: '',
        isDefault: false,
        added: 0,
        deleted: 0,
        mergeStrategy: 'squash',
        canMergeLocally: true,
        mergeConflicts: false,
        parentBranch: '',
        prUrl: '',
        prTitle: '',
        prTargetBranch: '',
        localPath: '',
        heldByPath: '',
        owningChatId: 'c1',
      } satisfies WorkspaceDTO,
    ])
  })

  it('yields ONE workspace per worktree even when several chats share it', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse([
        chatRow({ worktree: worktree() }),
        // A thread of c1 — same workspace, same worktree object, same owner.
        chatRow({ id: 'c2', parentId: 'c1', type: 'chat', worktree: worktree() }),
        // A second thread, two levels down. Still c1's worktree.
        chatRow({ id: 'c3', parentId: 'c2', type: 'chat', worktree: worktree() }),
        // A bubble that holds nothing.
        chatRow({ id: 'c4', workspaceId: '', type: 'chat' }),
        // A second worktree, with its own owning row.
        chatRow({
          id: 'c5',
          workspaceId: 'w2',
          type: 'branch',
          worktree: worktree({ branch: 'feature/y', owningChatId: 'c5' }),
        }),
      ]),
    )

    const result = await fetchWorkspaces('p1', 'r1')
    expect(result.map((ws) => ws.id)).toEqual(['w1', 'w2'])
    expect(result.map((ws) => ws.owningChatId)).toEqual(['c1', 'c5'])
    expect(result.map((ws) => ws.branch)).toEqual(['feature/x', 'feature/y'])
  })

  it('returns an empty list when the repo has no chats at all', async () => {
    fetchMock.mockResolvedValue(jsonResponse(null))
    await expect(fetchWorkspaces('p1', 'r1')).resolves.toEqual([])
  })
})

describe('fetchWorkspace', () => {
  afterEach(() => {
    __resetWorkspaceScopesForTest()
  })

  it('GETs the OWNING CHAT and maps its worktree', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'c1' })
    fetchMock.mockResolvedValue(jsonResponse(chatRow({ worktree: worktree() })))

    const result = await fetchWorkspace('p1', 'r1', 'w1')
    const [url] = fetchMock.mock.calls[0] as [string]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/c1')
    expect(result).toMatchObject({ id: 'w1', owningChatId: 'c1', branch: 'feature/x' })
  })

  it('throws when no owning chat was ever recorded, rather than guessing a URL', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w-orphan' })
    await expect(fetchWorkspace('p1', 'r1', 'w-orphan')).rejects.toThrow(/no owning chat/)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('throws when the chat came back holding no worktree', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'c1' })
    fetchMock.mockResolvedValue(jsonResponse(chatRow({ workspaceId: '', worktree: undefined })))
    await expect(fetchWorkspace('p1', 'r1', 'w1')).rejects.toThrow(/holds no worktree/)
  })
})

describe('apiFetch 202 handling', () => {
  it('treats a 202 Accepted with no body as success (undefined, no throw)', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 202 }))
    await expect(apiFetch('/v0/projects/p1/repos', { method: 'POST' })).resolves.toBeUndefined()
  })
})
