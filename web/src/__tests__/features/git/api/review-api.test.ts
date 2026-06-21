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
import type { ThreadDTO } from '@/features/git/api/review-api'
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

// Build a minimal real ThreadDTO (matches the live /threads backend shape).
function wireThreadDTO(overrides: Partial<ThreadDTO> = {}): ThreadDTO {
  return {
    id: 't1',
    projectId: 'p1',
    repoId: 'r1',
    workspaceId: 'ws-1',
    filePath: 'README.md',
    line: 10,
    startLine: 8,
    endLine: 12,
    side: 'new',
    body: 'root comment',
    author: 'char2cs',
    isAgent: false,
    resolved: false,
    createdAt: '2026-01-01T00:00:00Z',
    replies: [],
    ...overrides,
  }
}

describe('mapThread', () => {
  it('maps real ThreadDTO fields: line→lineNumber, resolved→isResolved, body+replies→messages', () => {
    const dto: ThreadDTO = {
      id: 'thread-1',
      projectId: 'p1',
      repoId: 'r1',
      workspaceId: 'ws-1',
      filePath: 'README.md',
      line: 7,
      startLine: 7,
      endLine: 8,
      side: 'new',
      body: 'hi **x**',
      author: 'char2cs',
      isAgent: false,
      resolved: false,
      createdAt: '2026-06-18T00:00:00Z',
      replies: [
        {
          id: 'r1',
          threadId: 'thread-1',
          body: 'reply',
          author: 'claude',
          isAgent: true,
          createdAt: '2026-06-18T01:00:00Z',
        },
      ],
    }

    const result = mapThread(dto)

    expect(result.lineNumber).toBe(7)
    expect(result.startLine).toBe(7)
    expect(result.endLine).toBe(8)
    expect(result.side).toBe('new')
    expect(result.isResolved).toBe(false)
    expect(result.messages).toHaveLength(2)
    expect(result.messages[0].body).toBe('hi **x**')
    expect(result.messages[0].author).toBe('char2cs')
    expect(result.messages[0].isAgent).toBe(false)
    expect(result.messages[1].id).toBe('r1')
    expect(result.messages[1].body).toBe('reply')
    expect(result.messages[1].author).toBe('claude')
    expect(result.messages[1].isAgent).toBe(true)
  })

  it('derives isResolved from resolved===true', () => {
    expect(mapThread(wireThreadDTO({ resolved: true })).isResolved).toBe(true)
  })

  it('derives isResolved===false from resolved===false', () => {
    expect(mapThread(wireThreadDTO({ resolved: false })).isResolved).toBe(false)
  })

  it('falls back startLine/endLine to line when 0', () => {
    const result = mapThread(wireThreadDTO({ line: 5, startLine: 0, endLine: 0 }))
    expect(result.lineNumber).toBe(5)
    expect(result.startLine).toBe(5)
    expect(result.endLine).toBe(5)
  })

  it('root message has synthetic id of `{t.id}:root`', () => {
    const result = mapThread(wireThreadDTO({ id: 'abc' }))
    expect(result.messages[0].id).toBe('abc:root')
  })

  it('empty replies produces a single root message', () => {
    const result = mapThread(wireThreadDTO({ replies: [] }))
    expect(result.messages).toHaveLength(1)
  })

  it('maps side correctly', () => {
    expect(mapThread(wireThreadDTO({ side: 'old' })).side).toBe('old')
    expect(mapThread(wireThreadDTO({ side: 'new' })).side).toBe('new')
  })
})

describe('review-api request shapes', () => {
  it('getReview reads the workspace-scoped /review route and returns threads:[]', async () => {
    const fetchMock = mockFetchEnvelope({
      description: 'desc',
      mergeStrategy: 'squash',
      diff: { files: [] },
      // old /review composite shape — intentionally ignored
      threads: [{ id: 't-old', lineNumber: 1, status: 'open', messages: [] }],
      conversations: null,
    })

    const review = await getReview('ws-1')

    expect(fetchMock.mock.calls[0][0]).toContain('/v0/projects/p1/repos/r1/workspaces/ws-1/review')
    expect(review.mergeStrategy).toBe('squash')
    // threads must be [] — sourced from /threads not from /review
    expect(review.threads).toEqual([])
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
    const fetchMock = mockFetchEnvelope(wireThreadDTO({
      id: 't9',
      filePath: 'README.md',
      line: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
      body: 'note',
      author: undefined as unknown as string,
      replies: [],
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
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ id: 't9', body: 'reply body', author: 'me', isAgent: false, replies: [] }))

    await replyToThread('ws-4', 't 9', { author: 'me', isAgent: false, body: 'reply body' })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    // Must use /replies (plural), not /reply
    expect(url).not.toContain('/reply')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-4/threads/t%209/replies')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toMatchObject({ author: 'me', isAgent: false, body: 'reply body' })
  })

  it('replyToThread accepts a plain string body for backward compat', async () => {
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ id: 't9', body: 'plain reply', replies: [] }))

    await replyToThread('ws-4', 'tid', 'plain reply')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/threads/tid/replies')
    expect(JSON.parse(init.body as string)).toEqual({ body: 'plain reply' })
  })

  it('setThreadResolved PATCHes /threads/:id (not /review/threads) with isResolved', async () => {
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ resolved: true }))

    const thread = await setThreadResolved('ws-6', 'thread-abc', true)

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).not.toContain('/review/threads')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-6/threads/thread-abc')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ isResolved: true })
    expect(thread.isResolved).toBe(true)
  })

  it('setThreadResolved can pass false (two-way) to reopen a thread', async () => {
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ resolved: false }))

    const thread = await setThreadResolved('ws-6', 'thread-xyz', false)

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ isResolved: false })
    expect(thread.isResolved).toBe(false)
  })

  it('listThreads GETs /threads and returns mapped array', async () => {
    const fetchMock = mockFetchEnvelope([
      wireThreadDTO({ id: 'ta', side: 'old' }),
      wireThreadDTO({ id: 'tb', side: 'new', resolved: true }),
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
