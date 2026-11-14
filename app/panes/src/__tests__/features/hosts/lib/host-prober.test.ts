import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startHostProber } from '@/features/hosts/lib/host-prober'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { destroyHostRuntime, getHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const INTERVAL = 15_000

/** Each host's health answer, by host; a missing one never answers. */
let answers: Record<string, () => Promise<Response>>
let fetchMock: ReturnType<typeof vi.fn>
let stop: (() => void) | undefined

const healthy = async () =>
  new Response(JSON.stringify({ success: true, data: {} }), { status: 200 })
const refused = async () => new Response(null, { status: 401 })
const unreachable = async () =>
  new Response(null, { status: 502, headers: { 'x-crowbar-proxy': 'host-unreachable' } })
const probesOf = (hostId: string) =>
  fetchMock.mock.calls.filter(([url]) => url === `crowbar://${hostId}/v0/health`).length
const statusOf = (hostId: string) => getHostRuntime(hostId)?.getState().status

/** A host whose health read waits until the test releases it. */
function parked(): { answer: () => Promise<Response>; release: () => void } {
  let release!: () => void
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  return { answer: () => gate.then(healthy), release }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: async () => {} }
  answers = {}
  fetchMock = vi.fn((url: string) => {
    const hostId = /^crowbar:\/\/([^/]+)\//.exec(url)?.[1] ?? ''
    return answers[hostId]?.() ?? new Promise<Response>(() => {})
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  stop?.()
  stop = undefined
  for (const id of ['h1', 'h2']) destroyHostRuntime(id)
  resetHostWorld()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

describe('startHostProber', () => {
  it('probes every configured host at once, then one interval after each answer', async () => {
    pairRemoteHost('h1')
    pairRemoteHost('h2')
    answers = { h1: healthy, h2: healthy }

    stop = startHostProber()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))
    await vi.waitFor(() => expect(statusOf('h2')).toBe('up'))
    expect([probesOf('h1'), probesOf('h2')]).toEqual([1, 1])

    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect([probesOf('h1'), probesOf('h2')]).toEqual([2, 2])
  })

  // A host that hangs costs one request, however many intervals pass.
  it('never overlaps two probes of one host', async () => {
    pairRemoteHost('h1')
    const h1 = parked()
    answers = { h1: h1.answer }

    stop = startHostProber()
    await vi.advanceTimersByTimeAsync(INTERVAL * 3)
    expect(probesOf('h1')).toBe(1)

    h1.release()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))
    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(probesOf('h1')).toBe(2)
  })

  it('says a host it has never heard from is being checked, and keeps a known status while re-probing', async () => {
    pairRemoteHost('h1')
    let h1 = parked()
    answers = { h1: () => h1.answer() }

    stop = startHostProber()
    await vi.waitFor(() => expect(probesOf('h1')).toBe(1))
    expect(statusOf('h1')).toBe('probing')
    h1.release()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    h1 = parked()
    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(probesOf('h1')).toBe(2)
    expect(statusOf('h1')).toBe('up')
    h1.release()
  })

  // /v0/health is guarded over TLS: a revoked token is a 401 on the periodic probe.
  it('marks a host whose token is refused as needing repair', async () => {
    pairRemoteHost('h1')
    answers = { h1: refused }

    stop = startHostProber()

    await vi.waitFor(() => expect(statusOf('h1')).toBe('needs-repair'))
  })

  it('starts probing a host configured later, and stops on its removal', async () => {
    answers = { h1: healthy }
    stop = startHostProber()
    expect(fetchMock).not.toHaveBeenCalled()

    pairRemoteHost('h1')
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    useHostConfigStore.getState().removeHost('h1')
    await vi.advanceTimersByTimeAsync(INTERVAL * 2)
    expect(probesOf('h1')).toBe(1)
  })

  it('does not bring back the runtime of a host removed while its probe was out', async () => {
    pairRemoteHost('h1')
    const h1 = parked()
    answers = { h1: h1.answer }
    stop = startHostProber()
    await vi.waitFor(() => expect(probesOf('h1')).toBe(1))

    useHostConfigStore.getState().removeHost('h1')
    h1.release()
    await vi.advanceTimersByTimeAsync(INTERVAL)

    expect(getHostRuntime('h1')).toBeUndefined()
    expect(probesOf('h1')).toBe(1)
  })

  // A restarting daemon is back within seconds: its streams must not wait out a whole interval.
  it('probes a host a stream finds down again within a second, and sees it back at once', async () => {
    pairRemoteHost('h1')
    answers = { h1: healthy }
    stop = startHostProber()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    answers = { h1: unreachable }
    getHostRuntime('h1')!.getState().setStatus('down')
    await vi.advanceTimersByTimeAsync(1_000)
    expect(probesOf('h1')).toBe(2)
    expect(statusOf('h1')).toBe('down')

    answers = { h1: healthy }
    await vi.advanceTimersByTimeAsync(2_000)
    expect(probesOf('h1')).toBe(3)
    expect(statusOf('h1')).toBe('up')
    await vi.advanceTimersByTimeAsync(INTERVAL - 1)
    expect(probesOf('h1')).toBe(3)
  })

  it('backs a host that stays down off to the normal interval', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const probedAt: number[] = []
    pairRemoteHost('h1')
    answers = {
      h1: () => {
        probedAt.push(Date.now())
        return unreachable()
      },
    }
    stop = startHostProber()

    await vi.advanceTimersByTimeAsync(50_000)

    const gaps = probedAt.slice(1).map((at, i) => at - probedAt[i])
    expect(gaps).toEqual([1_000, 2_000, 4_000, 8_000, INTERVAL, INTERVAL])
    expect(statusOf('h1')).toBe('down')
  })

  // A host back up has recovered: its next outage starts from the first step again.
  it('restarts the backoff when a host that was down comes back', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const probedAt: number[] = []
    const sequence = [unreachable, unreachable, healthy, unreachable]
    pairRemoteHost('h1')
    answers = {
      h1: () => {
        probedAt.push(Date.now())
        return (sequence[probedAt.length - 1] ?? unreachable)()
      },
    }
    stop = startHostProber()

    await vi.advanceTimersByTimeAsync(20_000)

    const gaps = probedAt.slice(1).map((at, i) => at - probedAt[i])
    expect(gaps).toEqual([1_000, 2_000, INTERVAL, 1_000])
  })

  it('never overlaps two probes when a stream finds the host down mid-probe', async () => {
    pairRemoteHost('h1')
    answers = { h1: healthy }
    stop = startHostProber()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))
    const h1 = parked()
    answers = { h1: h1.answer }
    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(probesOf('h1')).toBe(2)

    getHostRuntime('h1')!.getState().setStatus('down')
    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(probesOf('h1')).toBe(2)

    h1.release()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))
  })

  // A removed host has no status: nothing may go on reading it as up.
  it('lets go of a removed host’s runtime, however it was removed', async () => {
    pairRemoteHost('h1')
    answers = { h1: healthy }
    stop = startHostProber()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    useHostConfigStore.setState({ hosts: [] })

    expect(getHostRuntime('h1')).toBeUndefined()
  })

  it('stops every probe when the app stops it', async () => {
    pairRemoteHost('h1')
    answers = { h1: healthy }
    stop = startHostProber()
    await vi.waitFor(() => expect(statusOf('h1')).toBe('up'))

    stop()
    await vi.advanceTimersByTimeAsync(INTERVAL * 2)

    expect(probesOf('h1')).toBe(1)
  })

  it('sends no probe before the saved hosts are registered with the desktop proxy', async () => {
    let registered!: () => void
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {
      invoke: () =>
        new Promise<void>((resolve) => {
          registered = resolve
        }),
    }
    vi.resetModules()
    const config = await import('@/features/hosts/stores/host-config-store')
    config.useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'h1', endpoint: 'h1:1', pairedFingerprint: '', clientId: 'c' }],
    })
    const prober = await import('@/features/hosts/lib/host-prober')
    answers = { h1: healthy }

    stop = prober.startHostProber()
    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(fetchMock).not.toHaveBeenCalled()

    registered()
    await vi.waitFor(() => expect(probesOf('h1')).toBe(1))
  })
})
