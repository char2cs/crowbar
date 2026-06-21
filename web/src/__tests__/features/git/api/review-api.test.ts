import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getReview,
  setMergeStrategy,
  mergeIntoParent,
  openThread,
  replyToThread,
  setThreadResolved,
  listThreads,
  mapThread,
} from '@/features/git/api/review-api'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// §3: workspace-scoped URLs are hierarchical; register scopes for the test wsIds.
beforeEach(() => {
  for (const wsId of ['ws-1', 'ws-2', 'ws-3', 'ws-4', 'ws-5', 'ws-6']) {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId })
  }
})

function mockFetchEnvelope(data: unknown): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ success: true, data }),
  }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

// Minimal wire thread with the new fields.
function wireThread(overrides: Partial<{
  id: string
  filePath: string
  lineNumber: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  status: 'open' | 'resolved'
  messages: null | { id: string; author?: string; isAgent: boolean; body: string; createdAt: string }[]
  createdAt: string
}> = {}) {
  return {
    id: 't1',
    wsId: 'ws-1',
    filePath: 'README.md',
    lineNumber: 10,
    startLine: 8,
    endLine: 12,
    side: 'new' as const,
    status: 'open' as const,
    messages: null,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('mapThread', () => {
  it('derives isResolved from status === "resolved" and defaults null messages to []', () => {
    expect(
      mapThread(wireThread({ id: 't1', side: 'new', status: 'open', messages: null })),
    ).toEqual({
      id: 't1',
      filePath: 'README.md',
      lineNumber: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
      messages: [],
      isResolved: false,
    })
  })

  it('maps status "resolved" → isResolved true', () => {
    expect(
      mapThread(wireThread({ status: 'resolved' })).isResolved,
    ).toBe(true)
  })

  it('maps reply isAgent and falls back lineNumber when startLine/endLine absent', () => {
    const result = mapThread({
      id: 't2',
      wsId: 'ws-1',
      filePath: 'a.ts',
      lineNumber: 5,
      startLine: 0,
      endLine: 0,
      side: 'old',
      status: 'open',
      messages: [
        { id: 'm1', author: 'me', isAgent: true, body: 'hi', createdAt: '2026-01-01T00:00:00Z' },
      ],
      createdAt: '2026-01-01T00:00:00Z',
    })
    expect(result.messages[0].isAgent).toBe(true)
    expect(result.side).toBe('old')
  })
})

describe('review-api request shapes', () => {
  it('getReview reads the workspace-scoped /review route and unwraps threads', async () => {
    const fetchMock = mockFetchEnvelope({
      description: 'desc',
      mergeStrategy: 'squash',
      diff: { files: [] },
      threads: [wireThread({ status: 'open', messages: [] })],
      conversations: null,
    })

    const review = await getReview('ws-1')

    expect(fetchMock.mock.calls[0][0]).toContain('/v0/projects/p1/repos/r1/workspaces/ws-1/review')
    expect(review.mergeStrategy).toBe('squash')
    expect(review.threads).toHaveLength(1)
    expect(review.threads[0].filePath).toBe('README.md')
    expect(review.conversations).toEqual([])
  })

  it('setMergeStrategy PATCHes /review with the strategy body', async () => {
    const fetchMock = mockFetchEnvelope({ mergeStrategy: 'rebase' })

    const result = await setMergeStrategy('ws-2', 'rebase')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-2/review')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ mergeStrategy: 'rebase' })
    expect(result).toBe('rebase')
  })

  it('openThread POSTs to first-class /threads (not /review/threads) with all fields', async () => {
    const fetchMock = mockFetchEnvelope(wireThread({
      id: 't9',
      filePath: 'README.md',
      lineNumber: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
      status: 'open',
      messages: [
        { id: 'm1', author: undefined, isAgent: false, body: 'note', createdAt: '2026-01-01T00:00:00Z' },
      ],
    }))

    const thread = await openThread('ws-3', {
      filePath: 'README.md',
      line: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
      body: 'note',
    })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    // Must NOT have /review prefix — first-class endpoint
    expect(url).not.toContain('/review/threads')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-3/threads')
    expect(init.method).toBe('POST')
    const body = JSON.parse(init.body as string)
    expect(body).toMatchObject({
      filePath: 'README.md',
      line: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
    })
    expect(thread.messages[0].body).toBe('note')
    expect(thread.startLine).toBe(8)
    expect(thread.endLine).toBe(12)
    expect(thread.side).toBe('new')
  })

  it('mergeIntoParent POSTs to /merge-into-parent with the strategy body', async () => {
    const fetchMock = mockFetchEnvelope(null)

    await mergeIntoParent('ws-5', 'squash')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-5/merge-into-parent')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ strategy: 'squash' })
  })

  it('replyToThread POSTs to /threads/:id/replies (plural) with encoded id', async () => {
    const fetchMock = mockFetchEnvelope(wireThread({ id: 't9', messages: [] }))

    await replyToThread('ws-4', 't 9', { author: 'me', isAgent: false, body: 'reply body' })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    // Must use /replies (plural), not /reply
    expect(url).not.toContain('/reply')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-4/threads/t%209/replies')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toMatchObject({ author: 'me', isAgent: false, body: 'reply body' })
  })

  it('replyToThread accepts a plain string body for backward compat', async () => {
    const fetchMock = mockFetchEnvelope(wireThread({ id: 't9', messages: [] }))

    await replyToThread('ws-4', 'tid', 'plain reply')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/threads/tid/replies')
    expect(JSON.parse(init.body as string)).toEqual({ body: 'plain reply' })
  })

  it('setThreadResolved PATCHes /threads/:id (not /review/threads) with isResolved', async () => {
    const fetchMock = mockFetchEnvelope(wireThread({ status: 'resolved' }))

    const thread = await setThreadResolved('ws-6', 'thread-abc', true)

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).not.toContain('/review/threads')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-6/threads/thread-abc')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ isResolved: true })
    expect(thread.isResolved).toBe(true)
  })

  it('setThreadResolved can pass false (two-way) to reopen a thread', async () => {
    const fetchMock = mockFetchEnvelope(wireThread({ status: 'open' }))

    const thread = await setThreadResolved('ws-6', 'thread-xyz', false)

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ isResolved: false })
    expect(thread.isResolved).toBe(false)
  })

  it('listThreads GETs /threads and returns mapped array', async () => {
    const fetchMock = mockFetchEnvelope([
      wireThread({ id: 'ta', side: 'old' }),
      wireThread({ id: 'tb', side: 'new', status: 'resolved' }),
    ])

    const threads = await listThreads('ws-1')

    const [url] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-1/threads')
    expect(url).not.toContain('/review')
    expect(threads).toHaveLength(2)
    expect(threads[0].id).toBe('ta')
    expect(threads[0].side).toBe('old')
    expect(threads[1].isResolved).toBe(true)
  })
})
