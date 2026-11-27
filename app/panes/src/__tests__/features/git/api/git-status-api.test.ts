import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readGitStatus } from '@/features/git/api/git-status-api'
import { setWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })
})

function mockFetchEnvelope(data: unknown): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ success: true, data }),
    })),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('readGitStatus', () => {
  it("normalises a clean tree's files:null to an empty array", async () => {
    mockFetchEnvelope({ branch: 'main', ahead: 0, behind: 0, files: null })

    const { status } = await readGitStatus('ws-1')

    expect(status).not.toBeNull()
    expect(status?.files).toEqual([])
  })

  it('passes through a populated files array', async () => {
    const files = [{ path: 'a.ts', status: 'modified', staged: false }]
    mockFetchEnvelope({ branch: 'main', ahead: 1, behind: 0, files })

    const { status } = await readGitStatus('ws-1')

    expect(status?.files).toEqual(files)
    expect(status?.branch).toBe('main')
  })

  it('returns no status and the reason when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: async () => ({ success: false, error: 'boom' }),
      })),
    )

    const result = await readGitStatus('ws-1')

    expect(result.status).toBeNull()
    expect(result.error).toEqual(expect.any(String))
  })
})
