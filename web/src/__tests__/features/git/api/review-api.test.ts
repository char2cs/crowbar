import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getReview,
  setMergeStrategy,
  openThread,
  replyToThread,
  mapThread,
} from '@/features/git/api/review-api'

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

describe('mapThread', () => {
  it('derives isResolved from status === "resolved" and defaults null messages to []', () => {
    expect(
      mapThread({
        id: 't1',
        wsId: 'ws-1',
        filePath: 'README.md',
        lineNumber: 110,
        side: 'right',
        status: 'open',
        messages: null,
        createdAt: '2026-01-01T00:00:00Z',
      }),
    ).toEqual({
      id: 't1',
      filePath: 'README.md',
      lineNumber: 110,
      side: 'right',
      messages: [],
      isResolved: false,
    })

    expect(
      mapThread({
        id: 't2',
        wsId: 'ws-1',
        filePath: 'a.ts',
        lineNumber: 1,
        side: 'left',
        status: 'resolved',
        messages: [
          {
            id: 'm1',
            author: 'me',
            isAgent: false,
            body: 'hi',
            createdAt: '2026-01-01T00:00:00Z',
          },
        ],
        createdAt: '2026-01-01T00:00:00Z',
      }).isResolved,
    ).toBe(true)
  })
})

describe('review-api request shapes', () => {
  it('getReview reads the workspace-scoped /review route and unwraps threads', async () => {
    const fetchMock = mockFetchEnvelope({
      description: 'desc',
      mergeStrategy: 'squash',
      diff: { files: [] },
      threads: [
        {
          id: 't1',
          filePath: 'README.md',
          lineNumber: 110,
          side: 'right',
          status: 'open',
          messages: [],
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
      conversations: null,
    })

    const review = await getReview('ws-1')

    expect(fetchMock.mock.calls[0][0]).toContain('/v0/workspaces/ws-1/review')
    expect(review.mergeStrategy).toBe('squash')
    expect(review.threads).toHaveLength(1)
    expect(review.threads[0].filePath).toBe('README.md')
    expect(review.conversations).toEqual([])
  })

  it('setMergeStrategy PATCHes /review with the strategy body', async () => {
    const fetchMock = mockFetchEnvelope({ mergeStrategy: 'rebase' })

    const result = await setMergeStrategy('ws-2', 'rebase')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/workspaces/ws-2/review')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ mergeStrategy: 'rebase' })
    expect(result).toBe('rebase')
  })

  it('openThread POSTs to /review/threads with the anchor + body', async () => {
    const fetchMock = mockFetchEnvelope({
      id: 't9',
      filePath: 'README.md',
      lineNumber: 110,
      side: 'right',
      status: 'open',
      messages: [
        { id: 'm1', author: null, isAgent: false, body: 'note', createdAt: '2026-01-01T00:00:00Z' },
      ],
      createdAt: '2026-01-01T00:00:00Z',
    })

    const thread = await openThread('ws-3', {
      filePath: 'README.md',
      lineNumber: 110,
      side: 'right',
      body: 'note',
    })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/workspaces/ws-3/review/threads')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toMatchObject({
      filePath: 'README.md',
      lineNumber: 110,
    })
    expect(thread.messages[0].body).toBe('note')
  })

  it('replyToThread POSTs to the thread reply route with an encoded id', async () => {
    const fetchMock = mockFetchEnvelope({
      id: 't9',
      filePath: 'README.md',
      lineNumber: 110,
      side: 'right',
      status: 'open',
      messages: [],
      createdAt: '2026-01-01T00:00:00Z',
    })

    await replyToThread('ws-4', 't 9', 'reply body')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/workspaces/ws-4/review/threads/t%209/reply')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ body: 'reply body' })
  })
})
