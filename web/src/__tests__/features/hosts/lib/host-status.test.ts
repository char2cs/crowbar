import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startHostStatus } from '@/features/hosts/lib/host-status'
import { destroyHostRuntime, getHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import { reportChannelFailure, resetConnectionState } from '@/lib/ws/connection-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const statusOf = (hostId: string) => getHostRuntime(hostId)?.getState().status

let stop: (() => void) | undefined

beforeEach(() => {
  resetConnectionState()
  resetHostWorld()
  pairRemoteHost('h1')
})

afterEach(() => {
  stop?.()
  stop = undefined
  destroyHostRuntime('h1')
  resetHostWorld()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

// Between two probes, the streams are the first to know.
describe('startHostStatus', () => {
  it('flags a stream’s identity change before the next probe, and the probe keeps it', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: async () => {} }
    let identityChanged = false
    const health = vi.fn(async () =>
      identityChanged
        ? new Response(null, {
            status: 502,
            headers: { 'x-crowbar-proxy': 'host-identity-changed' },
          })
        : new Response(JSON.stringify({ success: true, data: {} }), { status: 200 }),
    )
    vi.stubGlobal('fetch', health)
    stop = startHostStatus()
    await vi.waitFor(() => expect(health).toHaveBeenCalledTimes(1))
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    identityChanged = true
    reportChannelFailure(
      'crowbar://h1/v0/projects',
      proxyMarkerError('host-identity-changed', 'h1')!,
    )

    expect(health).toHaveBeenCalledTimes(1)
    expect(statusOf('h1')).toBe('needs-repair')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(health).toHaveBeenCalledTimes(2)
    expect(statusOf('h1')).toBe('needs-repair')
  })
})
