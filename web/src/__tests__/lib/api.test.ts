import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { apiFetch, ApiError, fetchFolders, fetchHomeWorkspace } from '@/lib/api'

// A retry config that runs instantly (no real backoff sleeps) so the suite stays
// fast while still exercising the real attempt-counting logic.
const fastRetry = (attempts: number) => ({
  attempts,
  baseDelayMs: 0,
  maxDelayMs: 0,
  sleep: () => Promise.resolve(),
})

function envelope(data: unknown) {
  return { ok: true, status: 200, json: async () => ({ success: true, data }) }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('apiFetch transient-transport retry', () => {
  // Root cause of the cold-start landing bug: at app launch the daemon sidecar's
  // unix socket may not be accepting yet, so the very first GETs (fetchProjects /
  // fetchRepos) reject at the transport layer ("Load failed"). Those reads are
  // idempotent and must be retried until the daemon answers — otherwise the route
  // crashes (uncaught fetchProjects) or silently shows "No repositories yet".
  it('retries an idempotent GET on transport rejection, then resolves', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValueOnce(envelope([{ id: 'p1' }]))
    vi.stubGlobal('fetch', fetchMock)

    const data = await apiFetch<Array<{ id: string }>>('/v0/projects', undefined, fastRetry(8))

    expect(data).toEqual([{ id: 'p1' }])
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  // An HTTP error status is a real daemon response, not a transport failure — it
  // must surface immediately (a 404 is terminal; a 500 is a real server error).
  // Retrying it would mask genuine errors behind the startup window.
  it('does NOT retry on an HTTP error status', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: async () => ({ success: false, error: 'nope' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/v0/projects/x', undefined, fastRetry(8))).rejects.toMatchObject({
      name: 'ApiError',
      status: 404,
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('carries a structured recovery code from an error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        statusText: 'Conflict',
        json: async () => ({
          success: false,
          error: 'delivery outcome is unknown',
          code: 'request_outcome_uncertain',
        }),
      }),
    )

    const error = (await apiFetch('/v0/ws/w1/chats/c1/prompts', {
      method: 'POST',
    }).catch((caught) => caught)) as ApiError
    expect(error).toMatchObject({
      status: 409,
      code: 'request_outcome_uncertain',
      message: 'delivery outcome is unknown',
    })
  })

  // Mutations are NOT idempotent: a transport failure on a POST/DELETE must not be
  // blindly retried (the daemon's async 202 model + subscribe-before-POST handles
  // mutation resilience explicitly). Only safe reads auto-retry.
  it('does NOT retry a non-GET method on transport rejection', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Load failed'))
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/v0/projects', { method: 'POST' }, fastRetry(8))).rejects.toThrow(
      'Load failed',
    )
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  // Bounded: if the daemon never comes up, retries give up and surface the
  // transport error rather than hanging forever.
  it('gives up after the configured attempts and throws the transport error', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Load failed'))
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/v0/projects', undefined, fastRetry(3))).rejects.toThrow('Load failed')
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it('returns data on a first-try success without retrying', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    const data = await apiFetch<{ ok: boolean }>(
      '/v0/system/prerequisites',
      undefined,
      fastRetry(8),
    )

    expect(data).toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  // ApiError stays exported & shaped for status-specific callers (isNotFoundError).
  it('throws ApiError carrying the status for empty-body error responses', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => null,
    })
    vi.stubGlobal('fetch', fetchMock)

    const err = (await apiFetch('/v0/projects', undefined, fastRetry(8)).catch(
      (e) => e,
    )) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(500)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('fetchHomeWorkspace', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches from the correct endpoint and returns JSON', async () => {
    const dto = { id: 'ws-home', projectId: 'p1', kind: 'home' }
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, data: dto }), { status: 200 }),
    )
    const result = await fetchHomeWorkspace('p1')
    expect(result).toEqual(dto)
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      expect.stringContaining('/v0/projects/p1/home'),
      expect.any(Object),
    )
  })

  it('throws on non-2xx response', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ success: false, error: 'not found' }), { status: 404 }),
    )
    await expect(fetchHomeWorkspace('p1')).rejects.toThrow()
  })
})

describe('fetchFolders', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // Task 34: the dedicated `/folders` resource was deleted from the backend
  // (11b72c72) — folders are Chat rows now, served only via .../chats/folders,
  // whose wire DTO names its text field `title`, not `name`. Both the route and
  // the reshape are load-bearing: hitting the dead route 404s, and reading `name`
  // off the real DTO renders every folder blank.
  it('GETs .../chats/folders and reshapes the title-named wire DTO into a FolderDTO', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: 'f1', parentId: 'f0', title: 'Spikes', order: 2 }],
        }),
        { status: 200 },
      ),
    )
    const folders = await fetchFolders('p1', 'r1')
    expect(vi.mocked(fetch).mock.calls[0][0]).toContain('/v0/projects/p1/repos/r1/chats/folders')
    expect(vi.mocked(fetch).mock.calls[0][0]).not.toContain('/repos/r1/folders')
    expect(folders).toEqual([
      { id: 'f1', repoId: 'r1', projectId: 'p1', parentId: 'f0', name: 'Spikes', order: 2 },
    ])
  })

  it('returns [] when the backend responds with no body', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
    expect(await fetchFolders('p1', 'r1')).toEqual([])
  })
})
