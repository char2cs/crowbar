import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  fetchRepos,
  fetchWorkspace,
  fetchWorkspaces,
  apiFetch,
  workspaceDTOFromChat,
  workspaceDTOFromWorktreeFrame,
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
  provisioning: 'provisioned',
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
  type: 'chat',
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
        folderId: 'folder-1',
        order: 4,
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
      provisioning: 'provisioned',
      owningChatId: 'c1',
      folderId: 'folder-1',
      order: 4,
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
      folderId: '',
      order: 0,
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

describe('workspaceDTOFromWorktreeFrame', () => {
  const frame = (over: Record<string, unknown> = {}) => ({
    chatId: 'c1',
    workspaceId: 'w1',
    repoId: 'r1',
    kind: 'worktree_state',
    worktree: worktree(),
    ...over,
  })

  it("maps the frame's worktree when it names the repo this caller subscribed", () => {
    expect(workspaceDTOFromWorktreeFrame(frame(), 'p1', 'r1')).toMatchObject({
      id: 'w1',
      repoId: 'r1',
      projectId: 'p1',
      branch: 'feature/x',
      owningChatId: 'c1',
    })
  })

  it('returns null for every kind that is not a worktree state', () => {
    expect(workspaceDTOFromWorktreeFrame(frame({ kind: 'turn_started' }), 'p1', 'r1')).toBeNull()
    expect(workspaceDTOFromWorktreeFrame(null, 'p1', 'r1')).toBeNull()
  })

  it('returns null for a NON-owning row sharing the same worktree', () => {
    expect(workspaceDTOFromWorktreeFrame(frame({ chatId: 'c2' }), 'p1', 'r1')).toBeNull()
  })

  // TestRegression: a repo-scoped chats socket does NOT only carry that repo's
  // frames. The daemon fans a frame that names NO repo out to every subscriber
  // on purpose (container.go's matchRepoOrUnscoped) so the live folder feed and
  // root bubbles survive. The PROJECT-HOME worktree has no repo either, so its
  // worktree_state reached every repo's socket — and this mapper stamped the
  // SUBSCRIPTION's own repo id onto it, minting the home workspace as that
  // repo's workspace. Its branch is '', so rows-from-repo.ts drew it as a
  // labelless `branch` row under every repo header, in every project, the
  // instant a home chat took a turn. Live-reported twice.
  it('returns null for a repo-less (project-home) worktree instead of claiming it', () => {
    expect(workspaceDTOFromWorktreeFrame(frame({ repoId: '' }), 'p1', 'r1')).toBeNull()
    // `omitempty` — the field is absent on the wire, not empty.
    const { repoId: _dropped, ...noRepoId } = frame()
    expect(workspaceDTOFromWorktreeFrame(noRepoId, 'p1', 'r1')).toBeNull()
  })

  it("returns null for a frame that names ANOTHER repo than this caller's", () => {
    expect(workspaceDTOFromWorktreeFrame(frame({ repoId: 'r2' }), 'p1', 'r1')).toBeNull()
  })
})

describe('fetchWorkspaces', () => {
  const dto = (over: Partial<WorkspaceDTO> = {}): WorkspaceDTO => ({
    provisioning: 'provisioned',
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
    folderId: '',
    order: 0,
    ...over,
  })

  it('GETs the real .../workspaces resource and returns it as-is', async () => {
    const workspaces = [dto()]
    fetchMock.mockResolvedValue(jsonResponse(workspaces))
    const result = await fetchWorkspaces('p1', 'r1')
    const [url] = fetchMock.mock.calls[0] as [string]
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces')
    expect(result).toEqual(workspaces)
  })

  // TestRegression: fetchWorkspaces used to derive WorkspaceDTOs from the
  // repo's chat list, on the theory that every worktree worth showing has a
  // chat to derive it from. That theory breaks for a workspace with NO chat
  // at all — a repo's own default checkout before anyone has chatted in it,
  // or a locked tracking branch nobody ever opened a conversation in — which
  // had no chat row to derive from and so never appeared anywhere, taking the
  // repo's own header row with it (rows-from-repo.ts mints that from the
  // default workspace). Reproduced live against real production data. The
  // real resource has no such blind spot: it reports the workspace with
  // owningChatId: '', which rows-from-repo.ts already renders as an unfolded
  // branch row.
  it('includes a workspace with no owning chat at all', async () => {
    const workspaces = [dto({ id: 'w1', isDefault: true, owningChatId: '' })]
    fetchMock.mockResolvedValue(jsonResponse(workspaces))
    const result = await fetchWorkspaces('p1', 'r1')
    expect(result).toEqual(workspaces)
  })

  it('returns an empty list when the repo has no workspaces at all', async () => {
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
