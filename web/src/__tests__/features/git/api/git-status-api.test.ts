import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getGitStatus, unstagePaths } from '@/features/git/api/git-status-api'
import { expectMigratingShown, expectReported, watchToasts } from '@/__tests__/__fixtures__/reports'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// git is addressed through the chat that owns the worktree, so the api resolves
// its base from the route-recorded scope's owningChatId. Register one for the
// test wsId.
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

describe('getGitStatus', () => {
  // The backend serialises a clean tree as `files: null`; downstream reads `files.length`.
  it("normalises a clean tree's files:null to an empty array", async () => {
    mockFetchEnvelope({ branch: 'main', ahead: 0, behind: 0, files: null })

    const status = await getGitStatus('ws-1')

    expect(status).not.toBeNull()
    expect(status?.files).toEqual([])
  })

  it('passes through a populated files array', async () => {
    const files = [{ path: 'a.ts', status: 'modified', staged: false }]
    mockFetchEnvelope({ branch: 'main', ahead: 1, behind: 0, files })

    const status = await getGitStatus('ws-1')

    expect(status?.files).toEqual(files)
    expect(status?.branch).toBe('main')
  })

  it('returns null when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: async () => ({ success: false, error: 'boom' }),
      })),
    )

    await expect(getGitStatus('ws-1')).resolves.toBeNull()
  })
})

function mockRefusal(status: number, error: string, code: string): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok: false, status, json: async () => ({ success: false, error, code }) })),
  )
}

describe('unstagePaths', () => {
  it('a refused unstage reports the daemon’s reason', async () => {
    const toasts = watchToasts()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mockRefusal(409, 'index.lock exists', 'conflict')

    await expect(unstagePaths('ws-1', ['a.ts'])).resolves.toBe(false)

    expectReported(toasts, 'git unstage failed', 'index.lock exists')
  })

  it('an unstage refused while the daemon migrates shows the migrating state, not a toast', async () => {
    const toasts = watchToasts()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mockRefusal(503, 'Crowbar is upgrading its data (step 2 of 7)', 'daemon_migrating')

    await expect(unstagePaths('ws-1', ['a.ts'])).resolves.toBe(false)

    expectMigratingShown(toasts)
  })
})
