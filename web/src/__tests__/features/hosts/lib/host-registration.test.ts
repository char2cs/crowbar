import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Host } from '@/features/hosts/types'

const host = (id: string): Host => ({
  id,
  name: id,
  endpoint: `${id}.lan:3737`,
  pairedFingerprint: 'ff',
  clientId: `client-${id}`,
})

let invoke: ReturnType<typeof vi.fn>
let fetchMock: ReturnType<typeof vi.fn>

/** A page load: fresh modules over what the last session saved. */
async function boot(saved: Host[]) {
  vi.resetModules()
  const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
  useHostConfigStore.setState({ hosts: saved, remoteEverPaired: saved.length > 0 })
  return import('@/features/hosts/lib/host-registration')
}

const registered = () =>
  invoke.mock.calls.filter(([cmd]) => cmd === 'register_remote_host').map(([, args]) => args)

beforeEach(() => {
  invoke = vi.fn(async () => undefined)
  ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
  fetchMock = vi.fn(
    async () => new Response(JSON.stringify({ success: true, data: [] }), { status: 200 }),
  )
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('hostsRegistered', () => {
  // The desktop proxy's registry lives in memory: after a restart it knows no host.
  it('registers every saved host with the desktop proxy, once per page', async () => {
    const { hostsRegistered } = await boot([host('h1'), host('h2')])

    await hostsRegistered()
    await hostsRegistered()

    expect(registered()).toEqual([
      { hostId: 'h1', endpoint: 'h1.lan:3737' },
      { hostId: 'h2', endpoint: 'h2.lan:3737' },
    ])
  })

  it('holds a remote project read until the hosts are registered', async () => {
    let finish!: () => void
    invoke.mockImplementation(
      (cmd: string) =>
        new Promise<void>((resolve) => {
          if (cmd !== 'register_remote_host') return resolve()
          finish = resolve
        }),
    )
    const { hostsRegistered } = await boot([host('h1')])
    void hostsRegistered()
    const { projectListTransport } = await import('@/lib/api/project-list-transport')

    const read = projectListTransport.read('h1')
    await vi.waitFor(() => expect(registered()).toHaveLength(1))
    expect(fetchMock).not.toHaveBeenCalled()

    finish()
    await read
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['crowbar://h1/v0/projects'])
  })

  it('a host the proxy refuses does not keep the others, or the app, waiting', async () => {
    invoke.mockImplementation(async (cmd: string, args: { hostId: string }) => {
      if (cmd === 'register_remote_host' && args.hostId === 'h1') throw new Error('bad id')
    })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { hostsRegistered } = await boot([host('h1'), host('h2')])

    await expect(hostsRegistered()).resolves.toBeUndefined()
    expect(registered()).toEqual([
      { hostId: 'h1', endpoint: 'h1.lan:3737' },
      { hostId: 'h2', endpoint: 'h2.lan:3737' },
    ])
  })

  it('is ready at once in a browser, which has no proxy to register with', async () => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    const { hostsRegistered } = await boot([host('h1')])

    await hostsRegistered()

    expect(invoke).not.toHaveBeenCalled()
  })
})
