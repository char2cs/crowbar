import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import {
  postProject,
  postRepo,
  renameRepo,
  importBranches,
  deleteWorkspace,
  apiFetch,
} from '@/lib/api'

// §3/§7: every entity mutation is hierarchical and fire-and-forget — the daemon
// answers 202 Accepted with an EMPTY body and the real entity arrives over the
// scoped WS broadcaster. These tests pin the URLs/methods and that the client
// treats a 202-no-body as success returning undefined (never a synchronous id).
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function lastCall(): [string, RequestInit] {
  return fetchMock.mock.calls[fetchMock.mock.calls.length - 1] as [string, RequestInit]
}

describe('hierarchical mutations are 202-empty (no synchronous entity)', () => {
  test('POST /v0/projects → 202, resolves undefined', async () => {
    const res = await postProject('my-proj', '/tmp/my-proj')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'my-proj', path: '/tmp/my-proj' })
    expect(res).toBeUndefined()
  })

  test('POST /v0/projects/:p/repos → 202, resolves undefined', async () => {
    const res = await postRepo('p1', 'crowbar', '/tmp/crowbar')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'crowbar', path: '/tmp/crowbar' })
    expect(res).toBeUndefined()
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
  test('POST .../chats/import-batch body {branches} → 202, resolves undefined', async () => {
    const res = await importBranches('p1', 'r1', ['feature/x', 'feature/y'])
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos/r1/chats/import-batch')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ branches: ['feature/x', 'feature/y'] })
    expect(res).toBeUndefined()
  })

  // It stays ONE call over the set rather than N single-branch creates. Only the
  // batch route resolves the repo's open-PR graph ACROSS the branches, creates
  // the ancestors they are parented under, and falls back to a placeholder row
  // for a branch another worktree already holds; a loop would drop all three
  // silently, and answer 202 while doing so.
  test('sends every branch in ONE request, never one request per branch', async () => {
    await importBranches('p1', 'r1', ['a', 'b', 'c'])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [, init] = lastCall()
    expect(JSON.parse(init.body as string).branches).toEqual(['a', 'b', 'c'])
  })

  test('DELETE .../workspaces/:w → 202, resolves undefined', async () => {
    const res = await deleteWorkspace('p1', 'r1', 'w1')
    const [url, init] = lastCall()
    expect(url).toBe('/v0/projects/p1/repos/r1/workspaces/w1')
    expect(init.method).toBe('DELETE')
    expect(res).toBeUndefined()
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
