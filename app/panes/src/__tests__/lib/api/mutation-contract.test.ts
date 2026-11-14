import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { postProject, postRepo, renameRepo, importBranches, renameWorkspaceBranch } from '@/lib/api'
import { apiFetch } from '@/lib/api/fetch'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

// §3/§7: every entity mutation is hierarchical and fire-and-forget — the daemon
// answers 202 Accepted with an EMPTY body and the real entity arrives over the
// scoped WS broadcaster. These tests pin the URLs/methods and that the client
// treats a 202-no-body as success returning undefined (never a synchronous id).
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  vi.unstubAllGlobals()
  __resetWorkspaceScopesForTest()
})

function lastCall(): [string, RequestInit] {
  return fetchMock.mock.calls[fetchMock.mock.calls.length - 1] as [string, RequestInit]
}

describe('hierarchical mutations', () => {
  test('POST /v0/projects sends its operation id', async () => {
    await postProject('my-proj', '/tmp/my-proj', 'op-1', 'local')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('op-1')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'my-proj', path: '/tmp/my-proj' })
  })

  test('POST /v0/projects/:p/repos sends its operation id', async () => {
    await postRepo('p1', 'crowbar', '/tmp/crowbar', 'op-2')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('op-2')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'crowbar', path: '/tmp/crowbar' })
  })

  test('PATCH /v0/projects/:p/repos/:r body {name} → 204, resolves undefined', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const res = await renameRepo('p1', 'r1', 'New Name')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos/r1')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'New Name' })
    expect(res).toBeUndefined()
  })

  // Batch import moved onto the chat surface with the rest of creation: a
  // worktree is created and held by a chat, never by a route of its own.
  test('POST .../chats/import-batch sends each branch with its chat id and lock, and the operation key', async () => {
    const branches = [
      { name: 'feature/x', chatId: 'c1', lock: true },
      { name: 'feature/y', chatId: 'c2', lock: false },
    ]
    await importBranches('p1', 'r1', branches, 'op1')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/import-batch')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('op1')
    expect(JSON.parse(init.body as string)).toEqual({ branches })
  })

  // It stays ONE call over the set rather than N single-branch creates. Only the
  // batch route resolves the repo's open-PR graph ACROSS the branches, creates
  // the ancestors they are parented under, and falls back to a placeholder row
  // for a branch another worktree already holds; a loop would drop all three
  // silently, and answer 202 while doing so.
  test('sends every branch in ONE request, never one request per branch', async () => {
    const branches = ['a', 'b', 'c'].map((name) => ({ name, chatId: `chat-${name}`, lock: false }))
    await importBranches('p1', 'r1', branches, 'op1')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [, init] = lastCall()
    expect(JSON.parse(init.body as string).branches).toEqual(branches)
  })

  // A branch rename is a WORKTREE verb, so it is addressed by the chat holding
  // it — beside lock/merge/reparent on the same repo-scoped chat prefix, not on
  // a workspace route of its own. The old PATCH .../workspaces/:w also relocated
  // the worktree directory; this one renames the git ref and nothing else.
  test('PATCH .../chats/:chatId/branch body {branch}', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'c1' })
    await renameWorkspaceBranch('p1', 'r1', 'w1', 'feature/renamed')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/c1/branch')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ branch: 'feature/renamed' })
  })

  test('refuses to rename a workspace whose owning chat was never recorded', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w-orphan' })
    expect(() => renameWorkspaceBranch('p1', 'r1', 'w-orphan', 'x')).toThrow(/no owning chat/)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('apiFetch empty/204/202 success handling', () => {
  test('treats a 204 No Content success as undefined, not an error', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await expect(apiFetch('/v0/things/delete', { method: 'POST' })).resolves.toBeUndefined()
  })

  test('treats a 202 Accepted with no body as undefined', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 202 }))
    await expect(apiFetch('/v0/things/accept', { method: 'POST' })).resolves.toBeUndefined()
  })

  test('still throws on a real error envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: false, error: 'kaboom', data: null }), {
        status: 500,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await expect(apiFetch('/v0/things/boom')).rejects.toThrow('kaboom')
  })
})
