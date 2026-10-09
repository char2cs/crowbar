import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { probeHost } from '@/features/hosts/lib/host-probe'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const health = (status: number, headers: Record<string, string> = {}) =>
  new Response(status === 200 ? JSON.stringify({ success: true, data: {} }) : null, {
    status,
    headers,
  })

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
  pairRemoteHost('h1')
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  resetHostWorld()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

describe('probeHost', () => {
  it('reads the host’s own /v0/health once and says it is up, with how long it took', async () => {
    fetchMock.mockResolvedValue(health(200))

    const probe = await probeHost('h1')

    expect(probe).toEqual({ status: 'up', latencyMs: expect.any(Number) })
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['crowbar://h1/v0/health'])
  })

  it('says down when the proxy cannot reach the host, and does not retry', async () => {
    fetchMock.mockResolvedValue(health(503, { 'x-crowbar-proxy': 'host-unreachable' }))

    expect(await probeHost('h1')).toEqual({ status: 'down' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('says needs-repair when the host presents another identity', async () => {
    fetchMock.mockResolvedValue(health(502, { 'x-crowbar-proxy': 'host-identity-changed' }))

    expect(await probeHost('h1')).toEqual({ status: 'needs-repair' })
  })

  // /v0/health is guarded over TLS: a revoked token answers 401 on the periodic probe.
  it('says needs-repair when the host refuses the token', async () => {
    fetchMock.mockResolvedValue(health(401))

    expect(await probeHost('h1')).toEqual({ status: 'needs-repair' })
  })

  it('says down when the host never answers, without waiting on the proxy’s own timeout', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    fetchMock.mockImplementation(
      (_url: string, init?: RequestInit) =>
        new Promise((_resolve, reject) =>
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('aborted', 'AbortError')),
          ),
        ),
    )

    const probe = probeHost('h1')
    await vi.advanceTimersByTimeAsync(5_000)

    expect(await probe).toEqual({ status: 'down' })
  })
})
