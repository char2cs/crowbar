import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getReview,
  openThread,
  replyToThread,
  setThreadResolved,
  deleteThread,
  deleteMessage,
  editMessage,
  mapThread,
} from '@/features/git/api/review-api'
import type { ThreadDTO } from '@/lib/types'
import { setWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  for (const wsId of ['ws-1', 'ws-2', 'ws-3', 'ws-4', 'ws-5', 'ws-6']) {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId, owningChatId: `chat-of-${wsId}` })
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
    messageId: 'm0',
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
      messageId: 'm0',
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

  it('root message uses the real messageId from the wire', () => {
    const result = mapThread(wireThreadDTO({ id: 'abc', messageId: 'root-real' }))
    expect(result.messages[0].id).toBe('root-real')
  })

  it('root message falls back to synthetic `{t.id}:root` when messageId is empty', () => {
    const result = mapThread(wireThreadDTO({ id: 'abc', messageId: '' }))
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

  it('carries providerId + chatId on the root and on each reply', () => {
    const result = mapThread(
      wireThreadDTO({
        isAgent: true,
        providerId: 'zeta-cli',
        chatId: 'chat-1',
        replies: [
          {
            id: 'r1',
            threadId: 't1',
            body: 'reply',
            author: '',
            isAgent: true,
            providerId: 'zeta-cli',
            chatId: 'chat-2',
            createdAt: '2026-01-01T01:00:00Z',
          },
        ],
      }),
    )

    expect(result.messages[0]).toMatchObject({ providerId: 'zeta-cli', chatId: 'chat-1' })
    expect(result.messages[1]).toMatchObject({ providerId: 'zeta-cli', chatId: 'chat-2' })
  })

  it('leaves attribution undefined when the wire omits it or sends it empty', () => {
    const omitted = mapThread(wireThreadDTO())
    expect(omitted.messages[0].providerId).toBeUndefined()
    expect(omitted.messages[0].chatId).toBeUndefined()

    const empty = mapThread(wireThreadDTO({ providerId: '', chatId: '' }))
    expect(empty.messages[0].providerId).toBeUndefined()
    expect(empty.messages[0].chatId).toBeUndefined()
  })
})

describe('review-api request shapes', () => {
  it('getReview reads the chat-scoped /review route and returns threads:[]', async () => {
    const fetchMock = mockFetchEnvelope({
      description: 'desc',
      mergeStrategy: 'squash',
      diff: { files: [] },
      threads: [{ id: 't-old', lineNumber: 1, status: 'open', messages: [] }],
      conversations: null,
    })

    const review = await getReview('ws-1')

    expect(fetchMock.mock.calls[0][0]).toContain('/v0/chats/chat-of-ws-1/review')
    expect(review.mergeStrategy).toBe('squash')
    expect(review).not.toHaveProperty('threads')
    expect(review.conversations).toEqual([])
  })

  it('openThread POSTs to first-class /threads (not /review/threads) with all fields', async () => {
    const fetchMock = mockFetchEnvelope(
      wireThreadDTO({
        id: 't9',
        filePath: 'README.md',
        line: 10,
        startLine: 8,
        endLine: 12,
        side: 'new',
        body: 'note',
        author: undefined as unknown as string,
        replies: [],
      }),
    )

    const thread = await openThread('ws-3', {
      filePath: 'README.md',
      line: 10,
      startLine: 8,
      endLine: 12,
      side: 'new',
      body: 'note',
    })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
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

  it('replyToThread POSTs to /threads/:id/replies (plural) with encoded id', async () => {
    const fetchMock = mockFetchEnvelope(
      wireThreadDTO({ id: 't9', body: 'reply body', author: 'me', isAgent: false, replies: [] }),
    )

    await replyToThread('ws-4', 't 9', { author: 'me', isAgent: false, body: 'reply body' })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).not.toContain('/reply')
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-4/threads/t%209/replies')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toMatchObject({
      author: 'me',
      isAgent: false,
      body: 'reply body',
    })
  })

  it('replyToThread accepts a plain string body for backward compat', async () => {
    const fetchMock = mockFetchEnvelope(
      wireThreadDTO({ id: 't9', body: 'plain reply', replies: [] }),
    )

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

  it('deleteThread DELETEs /threads/:id with the encoded id', async () => {
    const fetchMock = mockFetchEnvelope({ id: 'thread-abc', deleted: true })

    await deleteThread('ws-6', 'thread abc')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-6/threads/thread%20abc')
    expect(url).not.toContain('/messages')
    expect(init.method).toBe('DELETE')
  })

  it('deleteMessage DELETEs /threads/:id/messages/:mid and returns the mapped thread', async () => {
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ id: 't9', replies: [] }))

    const thread = await deleteMessage('ws-6', 't9', 'm 2')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-6/threads/t9/messages/m%202')
    expect(init.method).toBe('DELETE')
    expect(thread.id).toBe('t9')
  })

  it('editMessage PATCHes /threads/:id/messages/:mid with the body and returns the mapped thread', async () => {
    const fetchMock = mockFetchEnvelope(wireThreadDTO({ id: 't9', body: 'edited' }))

    const thread = await editMessage('ws-6', 't9', 'm1', 'edited')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v0/projects/p1/repos/r1/workspaces/ws-6/threads/t9/messages/m1')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ body: 'edited' })
    expect(thread.messages[0].body).toBe('edited')
  })
})
