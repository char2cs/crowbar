import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { hydrateCriticalStores } from '@/lib/boot'
import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { Host } from '@/features/hosts/types'
import { dataOf, idle, type Loadable } from '@/lib/loadable'
import { saveCache } from '@/lib/persistence/cache-store'
import { resetDB } from '@/lib/persistence/idb'
import { useProjectDataStore } from '@/lib/store/projects'
import { getHomeTree, useHomeTreeStore } from '@/lib/store/home-tree'
import {
  __resetHomeWorkspaceResolverForTest,
  getHomeWorkspaceId,
} from '@/features/workspace/lib/home-workspace-resolver'
import type { Repo } from '@/lib/store/sidebar'
import {
  __resetWorkspaceScopesForTest,
  getWorkspaceScope,
  recordWorkspaceScope,
} from '@/lib/workspace-scope'
import { projectOn } from '@/__tests__/__fixtures__/remote-host'

const remoteHost: Host = {
  id: 'h1',
  name: 'homelab',
  endpoint: 'h1.lan:3737',
  pairedFingerprint: 'ff',
  clientId: 'client-h1',
}

const { hydrateSidebar, hydrateWindowPaneLayout, placeRestoredChatMembers } = vi.hoisted(() => ({
  hydrateSidebar: vi.fn().mockResolvedValue(undefined),
  hydrateWindowPaneLayout: vi.fn().mockResolvedValue(undefined),
  placeRestoredChatMembers: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredChatMembers,
}))

const { workspaceListFetch, setRepos, workspaceListData } = vi.hoisted(() => ({
  workspaceListFetch: vi.fn().mockResolvedValue(undefined),
  setRepos: vi.fn(),
  workspaceListData: { current: { status: 'idle' } as Loadable<Repo[]> },
}))
vi.mock('@/lib/store/workspace-list', () => ({
  useWorkspaceListStore: {
    getState: () => ({ fetch: workspaceListFetch, data: workspaceListData.current }),
  },
}))
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: { getState: () => ({ setRepos }) },
}))

const testRepo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  workspaces: [],
}

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
    workspaceListData.current = { status: 'idle' }
  })

  it('hydrates preferences and pane layout', async () => {
    await hydrateCriticalStores()

    expect(hydrateWindowPaneLayout).toHaveBeenCalledTimes(1)
  })

  // The daemon placement of restored members is network: started once the
  // layout is in the store, never awaited before first paint.
  it('starts placing restored chat members after the layout, without awaiting it', async () => {
    const order: string[] = []
    hydrateWindowPaneLayout.mockImplementation(async () => {
      order.push('layout')
    })
    placeRestoredChatMembers.mockImplementation(() => {
      order.push('place')
      return new Promise<void>(() => {})
    })

    await hydrateCriticalStores()

    expect(order).toEqual(['layout', 'place'])
  })

  it('retires the storage earlier builds left behind', async () => {
    localStorage.setItem('crowbar:settings:editorEngine', '"monaco"')

    await hydrateCriticalStores()

    expect(localStorage.getItem('crowbar:settings:editorEngine')).toBeNull()
  })

  it('sets repos from the workspace-list fetch before hydrating the sidebar', async () => {
    const order: string[] = []
    workspaceListFetch.mockImplementation(async () => {
      workspaceListData.current = { status: 'success', data: [testRepo], fetchedAt: Date.now() }
      order.push('fetch')
    })
    setRepos.mockImplementation(() => order.push('setRepos'))
    hydrateSidebar.mockImplementation(async () => {
      order.push('hydrateSidebar')
    })

    await hydrateCriticalStores()

    expect(setRepos).toHaveBeenCalledWith([testRepo])
    expect(order).toEqual(['fetch', 'setRepos', 'hydrateSidebar'])
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
      expect(placeRestoredChatMembers).not.toHaveBeenCalled()
    } finally {
      delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    }
  })

  it('resolves only after every step has completed — no step is fire-and-forget', async () => {
    let workspaceListResolved = false
    workspaceListFetch.mockImplementation(
      () =>
        new Promise<void>((resolve) =>
          setTimeout(() => {
            workspaceListResolved = true
            resolve()
          }, 0),
        ),
    )

    await hydrateCriticalStores()

    expect(workspaceListResolved).toBe(true)
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
    placeRestoredChatMembers.mockImplementation(async () => {
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

// Boot awaits the restored chat surface's chunk. Collection has no test timeout: transform
// that graph here, so the tests below time their boot rather than a cold import.
await import('@/features/agent/components/agent-chat-pane-gate')

// A reload that keeps an `#/ide/...` hash has no route guard to load the list first: the
// cached rows are what let a restored pane resolve its host before the first commit.
describe('the cached project list at boot', () => {
  const PAGE = { success: true, data: { cursor: 0, oldestCursor: 0, hasMore: false, items: [] } }
  const MESSAGES = '/v0/projects/p-local/repos/r1/chats/c1/messages?limit=100'
  let fetchMock: ReturnType<typeof vi.fn>
  const requested = () => fetchMock.mock.calls.map(([url]) => String(url))

  beforeEach(async () => {
    vi.clearAllMocks()
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: async () => {} }
    globalThis.indexedDB = new IDBFactory()
    resetDB()
    useProjectDataStore.setState({ data: idle() })
    resetWindowPaneStoreForTests()
    chatLedgerStore.setState({ ledgers: {} })
    __resetWorkspaceScopesForTest()
    __resetHomeWorkspaceResolverForTest()
    useHomeTreeStore.setState({ trees: {} })
    recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'w1', owningChatId: 'o1' })
    fetchMock = vi.fn(async () => new Response(JSON.stringify(PAGE), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await saveCache('projects-data', 'projects-by-host', [
      projectOn('p-local', 'local'),
      projectOn('p-remote', 'h1'),
    ])
    hydrateWindowPaneLayout.mockImplementation(async () => {
      windowPaneStore.getState().paneActions.setActiveProject('p-local')
      windowPaneStore.getState().paneActions.openChat('c1', { workspaceId: 'w1' })
    })
  })
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    vi.unstubAllGlobals()
    useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  })

  it('lets a restored local chat make its first read during boot on a paired install', async () => {
    useHostConfigStore.setState({ hosts: [remoteHost], remoteEverPaired: true })

    await hydrateCriticalStores()

    await vi.waitFor(() => expect(requested()).toContain(MESSAGES))
    // Held, but stale until the list's own first read replaces them.
    expect(useProjectDataStore.getState().data.status).toBe('loading')
    expect(requested()).not.toContain('/v0/projects')
  })

  // The sidebar tree is read for the projects the list carries, so a local-only
  // install that never seeded it would draw only the active project's cache.
  // A restored home pane resolves its host from the workspace scope, which only
  // GET /home gives: the last answer has to be in place before the first commit.
  it('holds each known project’s cached home workspace and tree before boot settles', async () => {
    await saveCache('workspaces-data', 'home-workspace:p-local', {
      wsId: 'ws-home-cached',
      owningChatId: 'chat-home-cached',
      localPath: '/home/cached',
    })
    await saveCache('workspaces-data', 'home-tree:p-local', {
      chats: [{ id: 'c-home', repoId: '', title: 'Home chat', order: 0 }],
      folders: [],
    })

    await hydrateCriticalStores()

    expect(getHomeWorkspaceId('p-local')).toBe('ws-home-cached')
    expect(getWorkspaceScope('ws-home-cached')?.projectId).toBe('p-local')
    expect(getHomeTree('p-local').chats.map((c) => c.id)).toEqual(['c-home'])
  })

  it('holds the cached list where no remote host was ever paired, without reading the network', async () => {
    await hydrateCriticalStores()

    expect(useProjectDataStore.getState().data.status).toBe('loading')
    expect(dataOf(useProjectDataStore.getState().data)?.map((p) => p.id)).toEqual([
      'p-local',
      'p-remote',
    ])
    await vi.waitFor(() => expect(requested()).toContain(MESSAGES))
    expect(requested()).not.toContain('/v0/projects')
  })
})
