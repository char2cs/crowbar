import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// api.ts imports isTauri from crowbar-bridge; hoisted so it exists when that
// mock's factory runs (see host-base.test.ts).
const { isTauri } = vi.hoisted(() => ({ isTauri: vi.fn(() => true) }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

import { API_BASE, ApiError, apiFetch, apiFetchRaw, apiUrl, assetURL } from '@/lib/api'
import { routeToHost } from '@/lib/api/host-route'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

const noRetry = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

function envelope(data: unknown) {
  return { ok: true, status: 200, json: async () => ({ success: true, data }) }
}

function failure(status: number, marker?: string, body: unknown = null) {
  return {
    ok: false,
    status,
    statusText: 'Nope',
    headers: new Headers(marker ? { 'x-crowbar-proxy': marker } : {}),
    json: async () => body,
  }
}

function addHost(id: string) {
  useHostConfigStore.getState().addHost({
    id,
    name: id,
    endpoint: `${id}.example:3737`,
    pairedFingerprint: 'ff',
    clientId: `client-${id}`,
  })
}

beforeEach(() => {
  isTauri.mockReturnValue(true)
  useHostConfigStore.setState({ hosts: [] })
  addHost('h1')
  addHost('h2')
})

afterEach(() => {
  vi.unstubAllGlobals()
  useHostConfigStore.setState({ hosts: [] })
})

describe('routeToHost', () => {
  it('leaves an unqualified path local and untouched', () => {
    expect(routeToHost('/v0/projects')).toEqual({ hostId: 'local', path: '/v0/projects' })
  })

  it('splits a qualified path into its host and the daemon-relative path', () => {
    expect(routeToHost('crowbar://h1/v0/projects?x=1')).toEqual({
      hostId: 'h1',
      path: '/v0/projects?x=1',
    })
  })

  it('reads the desktop proxy spelling of the local daemon as local', () => {
    expect(routeToHost('crowbar://localhost/v0/projects')).toEqual({
      hostId: 'local',
      path: '/v0/projects',
    })
  })

  it('applies an explicit host to an unqualified path', () => {
    expect(routeToHost('/v0/projects', 'h1')).toEqual({ hostId: 'h1', path: '/v0/projects' })
  })

  it('accepts an explicit host that names the same host as the path', () => {
    expect(routeToHost('crowbar://h1/v0/projects', 'h1')).toEqual({
      hostId: 'h1',
      path: '/v0/projects',
    })
    expect(routeToHost('crowbar://localhost/v0/projects', 'local')).toEqual({
      hostId: 'local',
      path: '/v0/projects',
    })
  })

  it('refuses an explicit host that disagrees with the path, whichever way', () => {
    expect(() => routeToHost('crowbar://h1/v0/projects', 'h2')).toThrow(/h1.*h2/)
    expect(() => routeToHost('crowbar://h1/v0/projects', 'local')).toThrow(/h1.*local/)
    expect(() => routeToHost('crowbar://localhost/v0/projects', 'h1')).toThrow(/local.*h1/)
  })

  it.each([
    'CROWBAR://h1/v0/projects',
    ' crowbar://h1/v0/projects',
    'crowbar://H1/v0/projects',
    'crowbar://h1//v0/projects',
    'crowbar:///v0/projects',
  ])('rejects the malformed %j instead of reading it as local', (input) => {
    expect(() => routeToHost(input)).toThrow()
  })
})

describe('apiFetch host routing', () => {
  it('sends an unqualified path to the local base exactly as before', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope([]))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/v0/projects', undefined, noRetry)

    expect(fetchMock).toHaveBeenCalledExactlyOnceWith(`${API_BASE}/v0/projects`, undefined)
  })

  it('routes a qualified path to that host with the prefix stripped', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope([]))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('crowbar://h1/v0/projects/p1/repos', undefined, noRetry)

    expect(fetchMock).toHaveBeenCalledExactlyOnceWith(
      'crowbar://h1/v0/projects/p1/repos',
      undefined,
    )
  })

  it('routes a write the same way', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope({ id: 'x' }))
    vi.stubGlobal('fetch', fetchMock)
    const init = { method: 'POST', body: '{}' }

    await apiFetch('crowbar://h1/v0/projects', init, noRetry)

    expect(fetchMock).toHaveBeenCalledExactlyOnceWith('crowbar://h1/v0/projects', init)
  })

  it('still honours the explicit hostId argument for an unqualified path', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope([]))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/v0/projects', undefined, noRetry, 'h2')

    expect(fetchMock).toHaveBeenCalledExactlyOnceWith('crowbar://h2/v0/projects', undefined)
  })

  it('refuses to guess when the path and the hostId argument name different hosts', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('crowbar://h1/v0/projects', undefined, noRetry, 'h2')).rejects.toThrow(
      /h1.*h2/,
    )
    await expect(apiFetch('crowbar://h1/v0/projects', undefined, noRetry, 'local')).rejects.toThrow(
      /h1.*local/,
    )
    await expect(apiFetchRaw('crowbar://h1/v0/projects', undefined, noRetry, 'h2')).rejects.toThrow(
      /h1.*h2/,
    )
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('never falls back to local for a malformed qualified path or an unknown host', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('CROWBAR://h1/v0/projects', undefined, noRetry)).rejects.toThrow()
    await expect(apiFetch('crowbar://nobody/v0/projects', undefined, noRetry)).rejects.toThrow(
      /unknown host/,
    )
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not reach a remote host from outside the desktop app', async () => {
    isTauri.mockReturnValue(false)
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('crowbar://h1/v0/projects', undefined, noRetry)).rejects.toThrow(
      /desktop app/,
    )
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('retries a qualified idempotent read against the same host', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValueOnce(envelope(['ok']))
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      apiFetch('crowbar://h1/v0/projects', undefined, { ...noRetry, attempts: 2 }),
    ).resolves.toEqual(['ok'])
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      'crowbar://h1/v0/projects',
      'crowbar://h1/v0/projects',
    ])
  })

  it('keeps two hosts’ identical reads apart, and shares the same host’s however it is spelled', async () => {
    const answers: Array<(v: unknown) => void> = []
    const fetchMock = vi.fn(() => new Promise((resolve) => answers.push(resolve)))
    vi.stubGlobal('fetch', fetchMock)

    const reads = [
      apiFetch('/v0/projects', undefined, noRetry),
      apiFetch('crowbar://h1/v0/projects', undefined, noRetry),
      apiFetch('/v0/projects', undefined, noRetry, 'h1'),
      apiFetch('crowbar://h2/v0/projects', undefined, noRetry),
    ]

    expect(fetchMock.mock.calls.map((call) => (call as unknown[])[0])).toEqual([
      `${API_BASE}/v0/projects`,
      'crowbar://h1/v0/projects',
      'crowbar://h2/v0/projects',
    ])
    answers.forEach((answer) => answer(envelope([])))
    await Promise.all(reads)
  })
})

describe('ApiError.hostId', () => {
  it('names the host a qualified request went to, on an HTTP error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(failure(404, undefined, { error: 'no such project' })),
    )

    const err = await apiFetch('crowbar://h1/v0/projects/x', undefined, noRetry).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 404, hostId: 'h1', message: 'no such project' })
  })

  it('names the host on a proxy-marked failure, keeping the code the marker maps to', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(failure(502, 'host-identity-changed')))

    const err = await apiFetch('/v0/projects', undefined, noRetry, 'h2').catch((e) => e)

    expect(err).toMatchObject({ code: 'host_identity_changed', hostId: 'h2' })
  })

  it('names the host when the envelope itself reports failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({ success: false, error: 'refused' }),
      }),
    )

    const err = await apiFetch('crowbar://h1/v0/projects', { method: 'POST' }, noRetry).catch(
      (e) => e,
    )

    expect(err).toMatchObject({ message: 'refused', hostId: 'h1' })
  })

  it('says local for a request that went to the local daemon', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(failure(500, undefined, { error: 'boom' })))

    const err = await apiFetchRaw('/v0/projects', undefined, noRetry).catch((e) => e)

    expect(err).toMatchObject({ status: 500, hostId: 'local' })
  })
})

describe('apiUrl and assetURL', () => {
  it('resolves an unqualified path against the local base, byte for byte as before', () => {
    expect(apiUrl('/v0/projects/p/icon')).toBe(`${API_BASE}/v0/projects/p/icon`)
    expect(assetURL('/v0/projects/p/icon')).toBe(`${API_BASE}/v0/projects/p/icon`)
  })

  it('resolves a qualified path against that host’s base', () => {
    expect(apiUrl('crowbar://h1/v0/chats/c/attachments')).toBe(
      'crowbar://h1/v0/chats/c/attachments',
    )
    expect(assetURL('crowbar://h2/v0/projects/p/icon')).toBe('crowbar://h2/v0/projects/p/icon')
  })

  it('throws for a host that is not configured instead of dialling local', () => {
    expect(() => apiUrl('crowbar://nobody/v0/x')).toThrow(/unknown host/)
    expect(() => assetURL('crowbar://nobody/v0/x')).toThrow(/unknown host/)
  })
})
