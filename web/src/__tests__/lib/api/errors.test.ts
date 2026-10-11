import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiFetch, isDaemonUnavailableError } from '@/lib/api'
import {
  ApiError,
  isHostIdentityChangedError,
  isHostUnreachableError,
  streamFailureOf,
} from '@/lib/api/errors'

// What ws_open rejects with for a remote host (desktop/src-tauri/src/ws_bridge/remote.rs):
// `<x-crowbar-proxy value>: <reason>`. Tauri hands a string error to JS as a string.
describe('streamFailureOf', () => {
  it('names an identity change the way an HTTP request failing that way is named', () => {
    const err = streamFailureOf(
      'host-identity-changed: the certificate presented by box:3737 no longer matches',
      'h1',
    )

    expect(err).toBeInstanceOf(ApiError)
    expect(isHostIdentityChangedError(err)).toBe(true)
    expect(err).toMatchObject({ hostId: 'h1', status: 502 })
  })

  it('names an unreachable host, and tells it apart from an identity change', () => {
    const err = streamFailureOf('host-unreachable: cannot reach box:3737: connection refused', 'h1')

    expect(isHostUnreachableError(err)).toBe(true)
    expect(isHostIdentityChangedError(err)).toBe(false)
    expect(err).toMatchObject({ hostId: 'h1', status: 503 })
  })

  it('reads the marker off an Error as well as a bare string', () => {
    const err = streamFailureOf(new Error('host-unreachable: dial timed out'), 'h2')

    expect(isHostUnreachableError(err)).toBe(true)
    expect(err?.hostId).toBe('h2')
  })

  it.each([
    'ws upgrade: HTTP error: 401 Unauthorized',
    'unknown remote host h1',
    'no stored credential for this host — pair it again',
    'connect daemon socket: No such file or directory',
    'daemon-unavailable: the local daemon is starting',
    ' host-unreachable: leading space',
    'host-unreachable',
    'host-unreachable-ish: x',
    '',
  ])('does not make a failure out of %j', (message) => {
    expect(streamFailureOf(message, 'h1')).toBeNull()
  })

  it('ignores errors that are not text', () => {
    expect(streamFailureOf(undefined, 'h1')).toBeNull()
    expect(streamFailureOf({ type: 'error' }, 'h1')).toBeNull()
    expect(streamFailureOf(new Event('error'), 'h1')).toBeNull()
  })
})

// What the desktop proxy answers in the daemon's place (x-crowbar-proxy): a resolved response,
// not a fetch rejection. Named, and never retried — the proxy already spent the read budget,
// so with eight attempts allowed fetch still runs exactly once.
describe('apiFetch on a proxy-marked answer', () => {
  const eightAttempts = { attempts: 8, baseDelayMs: 0, maxDelayMs: 0, sleep: async () => {} }
  const PREDICATES = [isDaemonUnavailableError, isHostUnreachableError, isHostIdentityChangedError]

  afterEach(() => vi.unstubAllGlobals())

  function answer(status: number, headers: Record<string, string>, json: () => Promise<unknown>) {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status,
      statusText: 'proxy',
      headers: new Headers(headers),
      json,
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }
  const textPlain = async () => {
    throw new SyntaxError('text/plain')
  }

  it.each([
    [503, 'daemon-unavailable', 'daemon_unavailable', isDaemonUnavailableError],
    [502, 'host-unreachable', 'host_unreachable', isHostUnreachableError],
    [502, 'host-identity-changed', 'host_identity_changed', isHostIdentityChangedError],
  ])('names a %i marked %s as %s, and as nothing else', async (status, marker, code, named) => {
    const fetchMock = answer(status, { 'x-crowbar-proxy': marker }, textPlain)

    const err: unknown = await apiFetch('/v0/projects', undefined, eightAttempts).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status, code })
    for (const is of PREDICATES) expect(is(err)).toBe(is === named)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('leaves an unmarked 503 as a plain daemon answer', async () => {
    answer(503, {}, async () => ({ success: false, error: 'busy' }))

    const err: unknown = await apiFetch('/v0/projects', undefined, eightAttempts).catch((e) => e)

    expect(err).toMatchObject({ status: 503, code: undefined })
    expect(isDaemonUnavailableError(err)).toBe(false)
  })
})
