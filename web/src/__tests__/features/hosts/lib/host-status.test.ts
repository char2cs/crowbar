import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { hostStatusOf, onHostUp, startHostStatus } from '@/features/hosts/lib/host-status'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import {
  reportChannelFailure,
  reportChannelState,
  resetConnectionState,
} from '@/lib/ws/connection-store'
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

describe('hostStatusOf', () => {
  it('reads a remote host’s status from what the app last learned of it, never its channels', () => {
    expect(hostStatusOf('h1')).toBe('unknown')
    reportChannelState('crowbar://h1/v0/projects', true, 'h1')
    expect(hostStatusOf('h1')).toBe('unknown')

    getOrCreateHostRuntime('h1').getState().setStatus('needs-repair')
    expect(hostStatusOf('h1')).toBe('needs-repair')
  })

  // The local daemon is never probed: its own streams are all that say it is reachable.
  it('reads the local daemon’s status from its own streams', () => {
    expect(hostStatusOf('local')).toBe('unknown')
    reportChannelState('/v0/projects', true, 'local')
    expect(hostStatusOf('local')).toBe('up')
    reportChannelState('/v0/projects', false, 'local')
    expect(hostStatusOf('local')).toBe('down')
  })
})

// The one signal that a host is reachable again, for whatever waits on it (a terminal).
describe('onHostUp', () => {
  it('fires each time a remote host’s status becomes up; its channels opening say nothing', () => {
    const up = vi.fn()
    getOrCreateHostRuntime('h1').getState().setStatus('down')
    stop = onHostUp('h1', up)

    reportChannelState('crowbar://h1/v0/projects', true, 'h1')
    reportChannelState('/v0/projects', true, 'local')
    expect(up).not.toHaveBeenCalled()

    getHostRuntime('h1')!.getState().setStatus('up', 3)
    getHostRuntime('h1')!.getState().setStatus('up', 4)
    getHostRuntime('h1')!.getState().setStatus('needs-repair')
    getHostRuntime('h1')!.getState().setStatus('up', 5)
    expect(up).toHaveBeenCalledTimes(2)
  })

  it('fires for the local daemon when its streams come back, not when a remote host does', () => {
    reportChannelState('/v0/projects', false, 'local')
    const up = vi.fn()
    stop = onHostUp('local', up)

    getOrCreateHostRuntime('h1').getState().setStatus('up')
    expect(up).not.toHaveBeenCalled()

    reportChannelState('/v0/projects', true, 'local')
    expect(up).toHaveBeenCalledTimes(1)
  })

  it('follows a host whose status is first kept after it started waiting', () => {
    const up = vi.fn()
    stop = onHostUp('h1', up)

    getOrCreateHostRuntime('h1').getState().setStatus('up')

    expect(up).toHaveBeenCalledTimes(1)
  })

  it('fires nothing once stopped', () => {
    const up = vi.fn()
    onHostUp('h1', up)()

    getOrCreateHostRuntime('h1').getState().setStatus('up')

    expect(up).not.toHaveBeenCalled()
  })
})
