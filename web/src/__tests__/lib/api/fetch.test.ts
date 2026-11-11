import { afterEach, describe, expect, it, vi } from 'vitest'

const install = vi.hoisted(() => ({ localDaemon: true }))
vi.mock('@/features/hosts/lib/local-daemon', () => ({ hasLocalDaemon: () => install.localDaemon }))

import { apiFetch } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED, userWords } from '@/lib/errors/report'
import { noteDaemonChange } from '@/lib/transport/daemon-changes'
import { resetConnectionState, useConnectionStore } from '@/lib/ws/connection-store'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

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
  install.localDaemon = true
})

// The phone app: a request for the local daemon has nothing to reach, so it fails at
// once as the daemon being unavailable instead of retrying a socket that never exists.
describe('apiFetch with no local daemon', () => {
  it('fails a local request at once, without fetching', async () => {
    install.localDaemon = false
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    const read = apiFetch('/v0/projects', undefined, fastRetry(8))

    await expect(read).rejects.toMatchObject({ name: 'ApiError', status: 503, hostId: 'local' })
    expect(fetchMock).not.toHaveBeenCalled()
  })
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
      problem: { code: 'request_outcome_uncertain', message: 'delivery outcome is unknown' },
    })
  })

  // Mutations are NOT idempotent: a transport failure on a POST/DELETE must not be
  // blindly retried (the daemon's async 202 model + subscribe-before-POST handles
  // mutation resilience explicitly). Only safe reads auto-retry.
  it('does NOT retry a non-GET method on transport rejection', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Load failed'))
    vi.stubGlobal('fetch', fetchMock)

    const err: unknown = await apiFetch('/v0/projects', { method: 'POST' }, fastRetry(8)).catch(
      (e) => e,
    )

    expect(fetchMock).toHaveBeenCalledTimes(1)
    // The daemon may have applied it before the connection dropped: never invite a blind retry.
    expect(userWords(err)).toBe(
      'The connection dropped, so the change may or may not have been applied. Check before trying again.',
    )
    expect((err as Error).cause).toBeInstanceOf(TypeError)
  })

  // Bounded: if the daemon never comes up, retries give up and say so rather
  // than hanging forever.
  it('gives up after the configured attempts and says the daemon was not reached', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)

    const err: unknown = await apiFetch('/v0/projects', undefined, fastRetry(3)).catch((e) => e)

    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(userWords(err)).toBe(UNREACHED)
    expect((err as Error).cause).toBeInstanceOf(TypeError)
  })

  it('passes a cancelled request through as the abort it is', async () => {
    const abort = new DOMException('The operation was aborted.', 'AbortError')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(abort))

    await expect(apiFetch('/v0/projects', { method: 'POST' }, fastRetry(1))).rejects.toBe(abort)
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

  // The desktop proxy answers a connect-refused with its OWN 503 once its
  // connect retries are spent (api_proxy.rs, header x-crowbar-proxy:
  // daemon-unavailable). That is the daemon being absent, not a daemon
  // answer: named as such so UI copy can say "starting" — and NOT retried
  // here, since the proxy already spent the idempotent-read budget and a
  // second layer would compound to ~36s per GET against a dead daemon.
  it('names a proxy-marked 503 as daemon_unavailable, without retrying it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
      headers: new Headers({ 'x-crowbar-proxy': 'daemon-unavailable' }),
      json: async () => {
        throw new SyntaxError('text/plain')
      },
    })
    vi.stubGlobal('fetch', fetchMock)

    const err = (await apiFetch('/v0/projects', undefined, fastRetry(8)).catch(
      (e) => e,
    )) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(503)
    expect(err.problem.code).toBe('daemon_unavailable')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('leaves an unmarked 503 as a plain daemon answer', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        statusText: 'Service Unavailable',
        headers: new Headers(),
        json: async () => ({ success: false, error: 'busy' }),
      }),
    )
    const err = (await apiFetch('/v0/projects', undefined, fastRetry(8)).catch(
      (e) => e,
    )) as ApiError
    expect(err.problem).toEqual({ code: 'unavailable', message: 'busy' })
  })
})

// Boot asks for one list from two owners a few ms apart (a workspace's chat
// seed and the sidebar tree). Nothing the client can know of has changed in
// between, so the second ask is served by the first request.
describe('apiFetch shared reads', () => {
  function parkedFetch() {
    const answers: Array<() => void> = []
    const fetchMock = vi.fn(
      (_url: string) =>
        new Promise((resolve) => {
          answers.push(() => resolve(envelope([{ id: 'c1' }])))
        }),
    )
    vi.stubGlobal('fetch', fetchMock)
    return { fetchMock, answerAll: () => answers.forEach((answer) => answer()) }
  }

  it('an identical GET while one is in flight shares it, each caller getting its own copy', async () => {
    const { fetchMock, answerAll } = parkedFetch()
    const first = apiFetch<Array<{ id: string }>>('/v0/list', undefined, fastRetry(1))
    const second = apiFetch<Array<{ id: string }>>('/v0/list', undefined, fastRetry(1))
    answerAll()
    const [a, b] = await Promise.all([first, second])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(b).toEqual(a)
    expect(b).not.toBe(a)
  })

  it('a GET asked after a daemon frame or a write gets its own request', async () => {
    const { fetchMock, answerAll } = parkedFetch()
    const reads = [apiFetch('/v0/list', undefined, fastRetry(1))]
    noteDaemonChange()
    reads.push(apiFetch('/v0/list', undefined, fastRetry(1)))
    void apiFetch('/v0/other', { method: 'POST' }, fastRetry(1)).catch(() => {})
    reads.push(apiFetch('/v0/list', undefined, fastRetry(1)))
    answerAll()
    await Promise.all(reads)
    expect(
      fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/v0/list')),
    ).toHaveLength(3)
  })

  it('a settled read is not reused', async () => {
    const fetchMock = vi.fn(async () => envelope([]))
    vi.stubGlobal('fetch', fetchMock)
    await apiFetch('/v0/list', undefined, fastRetry(1))
    await apiFetch('/v0/list', undefined, fastRetry(1))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

describe('apiFetch refusals', () => {
  it('a refusal envelope becomes ApiError.problem with the daemon code and message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        statusText: 'Conflict',
        json: async () => ({ success: false, error: 'held', code: 'conflict' }),
      }),
    )
    const err = (await apiFetch('/v0/x', { method: 'POST' }).catch((e) => e)) as ApiError
    expect(err.problem).toEqual({ code: 'conflict', message: 'held' })
  })

  it('a refusal body that is not JSON becomes an internal problem naming the status', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response('<html>bad gateway</html>', {
          status: 502,
          statusText: 'Bad Gateway',
          headers: { 'Content-Type': 'text/html' },
        }),
      ),
    )
    const err = (await apiFetch('/v0/x', { method: 'POST' }).catch((e) => e)) as ApiError
    expect(err.status).toBe(502)
    expect(err.problem).toEqual({ code: 'internal', message: '502 Bad Gateway' })
  })

  it('a 200 envelope that refuses carries its code and data', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ success: false, error: 'busy', code: 'chat_busy', data: { n: 1 } }),
            { status: 200 },
          ),
        ),
    )
    const err = (await apiFetch('/v0/x', { method: 'POST' }).catch((e) => e)) as ApiError
    expect(err.problem).toEqual({ code: 'chat_busy', message: 'busy' })
    expect(err.data).toEqual({ n: 1 })
  })

  it('a daemon_migrating refusal reports the state before it throws', async () => {
    resetConnectionState()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(refusal('daemon_migrating', 'Crowbar is starting')),
    )

    const seen = await apiFetch('/v0/projects', undefined, fastRetry(1)).catch(
      () => useConnectionStore.getState().migrating,
    )

    expect(seen).toEqual({ message: 'Crowbar is starting' })
    resetConnectionState()
  })

  // A daemon that halts before the page loads announces it to nobody: the
  // first request it refuses is how the browser learns of the halt.
  it('a daemon_halted refusal halts the daemon before it throws', async () => {
    useDaemonHaltStore.getState().clear()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(refusal('daemon_halted', 'layout 9 is newer')))

    const seen = await apiFetch('/v0/projects', undefined, fastRetry(1)).catch(
      () => useDaemonHaltStore.getState().message,
    )

    expect(seen).toBe('layout 9 is newer')
    useDaemonHaltStore.getState().clear()
  })
})

function refusal(code: string, error: string): Response {
  return new Response(JSON.stringify({ success: false, error, code }), {
    status: 503,
    statusText: 'Service Unavailable',
  })
}
