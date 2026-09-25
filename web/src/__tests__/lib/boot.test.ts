import { describe, it, expect, vi, beforeEach } from 'vitest'
import { hydrateCriticalStores } from '@/lib/boot'
import type { Loadable } from '@/lib/loadable'
import type { Repo } from '@/lib/store/sidebar'

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
