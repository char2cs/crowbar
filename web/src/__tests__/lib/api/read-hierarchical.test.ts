import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { fetchRepos, fetchWorkspaces, apiFetch } from '@/lib/api'
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
  it('GETs the hierarchical workspaces URL and returns the WorkspaceDTO list', async () => {
    const workspaces: WorkspaceDTO[] = [
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
        added: 0,
        deleted: 0,
        mergeStrategy: '',
        canMergeLocally: false,
        parentBranch: '',
        prUrl: '',
        prTitle: '',
        prTargetBranch: '',
      },
    ]
    fetchMock.mockResolvedValue(jsonResponse(workspaces))
    const result = await fetchWorkspaces('p1', 'r1')
    const [url] = fetchMock.mock.calls[0] as [string]
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces')
    expect(result).toEqual(workspaces)
  })
})

describe('apiFetch 202 handling', () => {
  it('treats a 202 Accepted with no body as success (undefined, no throw)', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 202 }))
    await expect(apiFetch('/v0/projects/p1/repos', { method: 'POST' })).resolves.toBeUndefined()
  })
})
