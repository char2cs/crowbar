import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { fetchRepos, fetchWorkspaces } from '@/lib/api'
import { apiFetch } from '@/lib/api/fetch'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

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

  // Workspaces come from the real resource, not the chat list: a workspace with no chat (an
  // unchatted default checkout, an unopened locked branch) must still appear, owningChatId '',
  // or the repo header goes missing with it.
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

describe('apiFetch 202 handling', () => {
  it('treats a 202 Accepted with no body as success (undefined, no throw)', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 202 }))
    await expect(apiFetch('/v0/projects/p1/repos', { method: 'POST' })).resolves.toBeUndefined()
  })

  it('a 202 that carries the operation it started answers that operation', async () => {
    const operation = { id: 'op1', kind: 'git.push', state: 'accepted', version: 1 }
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: { operation } }), { status: 202 }),
    )
    await expect(apiFetch('/v0/chats/c1/git/push', { method: 'POST' })).resolves.toEqual({
      operation,
    })
  })
})
