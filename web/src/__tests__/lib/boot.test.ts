import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest'
import { hydrateCriticalStores } from '@/lib/boot'
import type { Host } from '@/features/hosts/types'

const remoteHost: Host = {
  id: 'h1',
  name: 'homelab',
  endpoint: 'h1.lan:3737',
  pairedFingerprint: 'ff',
  clientId: 'client-h1',
}

const { hydrateSidebar, hydrateWindowPaneLayout, placeRestoredPanesOnceSynced } = vi.hoisted(
  () => ({
    hydrateSidebar: vi.fn().mockResolvedValue(undefined),
    hydrateWindowPaneLayout: vi.fn().mockResolvedValue(undefined),
    placeRestoredPanesOnceSynced: vi.fn(),
  }),
)
vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredPanesOnceSynced,
}))

// Regression: this ordering used to live inside a React component
// (`HydrationGate`) that gated first paint on ALL of it, including the one
// genuine network call (`/v0/projects`) — live-measured as a 600-900ms blank
// screen on every cold boot. Moving it here, awaited in main.tsx BEFORE
// `renderApp()` is ever called, fixed that AND a real crash the naive "just
// render immediately" fix introduced: a `WorkspaceView`/`EditorSurface` that
// mounts against `windowPaneStore`'s just-booted defaults, only to have
// `hydrateWindowPaneLayout` replace the whole layout out from under it a
// frame later, threw ("Editor failed to load") on every retained pane.
// `hydrateCriticalStores` must therefore fully resolve — synchronously, from
// the caller's perspective — before React ever mounts.
describe('hydrateCriticalStores', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('hydrates preferences and pane layout', async () => {
    await hydrateCriticalStores()

    expect(hydrateWindowPaneLayout).toHaveBeenCalledTimes(1)
  })

  // Restored members are placed once the projects scope answers, never before first paint.
  it('arms placing restored chat members after the layout', async () => {
    const order: string[] = []
    hydrateWindowPaneLayout.mockImplementation(async () => {
      order.push('layout')
    })
    placeRestoredPanesOnceSynced.mockImplementation(() => {
      order.push('place')
    })

    await hydrateCriticalStores()

    expect(order).toEqual(['layout', 'place'])
  })

  it('retires the storage earlier builds left behind', async () => {
    localStorage.setItem('crowbar:settings:editorEngine', '"monaco"')

    await hydrateCriticalStores()

    expect(localStorage.getItem('crowbar:settings:editorEngine')).toBeNull()
  })

  it('hydrates the sidebar UI after the layout', async () => {
    const order: string[] = []
    hydrateWindowPaneLayout.mockImplementation(async () => {
      order.push('layout')
    })
    hydrateSidebar.mockImplementation(async () => {
      order.push('hydrateSidebar')
    })

    await hydrateCriticalStores()

    expect(order).toEqual(['layout', 'hydrateSidebar'])
  })

  it('registers nothing with the desktop proxy where no remote host was ever paired', async () => {
    const invoke = vi.fn(async () => undefined)
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
    try {
      await hydrateCriticalStores()
      expect(invoke).not.toHaveBeenCalled()
    } finally {
      delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    }
  })

  it('fails as it did when the layout read fails where no remote host was ever paired', async () => {
    const invoke = vi.fn(async () => undefined)
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
    hydrateWindowPaneLayout.mockRejectedValueOnce(new Error('IndexedDB unavailable'))
    try {
      await expect(hydrateCriticalStores()).rejects.toThrow('IndexedDB unavailable')
      expect(invoke).not.toHaveBeenCalled()
      expect(placeRestoredPanesOnceSynced).not.toHaveBeenCalled()
    } finally {
      delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    }
  })
})

describe('hydrateCriticalStores on an install that has paired a remote host', () => {
  let invoke: ReturnType<typeof vi.fn>

  /** A page load over a saved host list: fresh modules, as after a restart. */
  async function freshBoot() {
    vi.resetModules()
    const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
    useHostConfigStore.setState({ hosts: [remoteHost], remoteEverPaired: true })
    return (await import('@/lib/boot')).hydrateCriticalStores
  }

  beforeEach(() => {
    vi.clearAllMocks()
    invoke = vi.fn(async () => undefined)
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
  })
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    localStorage.clear()
  })

  // The proxy's registry starts empty at every launch: a remote read sent first is refused.
  it('registers every saved host before the first remote read goes out', async () => {
    const order: string[] = []
    let finish!: () => void
    invoke.mockImplementation(
      (cmd: string) =>
        new Promise<void>((resolve) => {
          if (cmd !== 'register_remote_host') return resolve()
          order.push('register')
          finish = () => {
            order.push('registered')
            resolve()
          }
        }),
    )
    placeRestoredPanesOnceSynced.mockImplementation(() => {
      order.push('place')
    })
    const hydrate = await freshBoot()

    const done = hydrate()
    await vi.waitFor(() => expect(order).toEqual(['register']))
    finish()
    await done

    expect(order).toEqual(['register', 'registered', 'place'])
    expect(invoke).toHaveBeenCalledWith('register_remote_host', {
      hostId: remoteHost.id,
      endpoint: remoteHost.endpoint,
    })
  })

  // main.tsx renders once boot settles, even when it fails: render must not overtake the
  // registration already under way, or a restored remote stream meets an unknown host.
  it('waits for the hosts to register even when the layout read fails', async () => {
    const order: string[] = []
    let finish!: () => void
    invoke.mockImplementation(
      (cmd: string) =>
        new Promise<void>((resolve) => {
          if (cmd !== 'register_remote_host') return resolve()
          finish = () => {
            order.push('registered')
            resolve()
          }
        }),
    )
    hydrateWindowPaneLayout.mockRejectedValueOnce(new Error('IndexedDB unavailable'))
    const hydrate = await freshBoot()

    const settled = () => order.push('settled')
    const done = hydrate().then(settled, settled)
    // A macrotask: every microtask the layout's rejection set off has run by now.
    await new Promise((resolve) => setTimeout(resolve, 0))
    finish()
    await done

    expect(order).toEqual(['registered', 'settled'])
  })
})
