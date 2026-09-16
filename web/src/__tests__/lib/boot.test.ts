import { describe, it, expect, vi, beforeEach } from 'vitest'
import { hydrateCriticalStores, hydrateProjectsInBackground } from '@/lib/boot'
import type { Loadable } from '@/lib/loadable'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

const { hydratePreferences, hydrateSidebar, hydrateWindowPaneLayout } = vi.hoisted(() => ({
  hydratePreferences: vi.fn().mockResolvedValue(null),
  hydrateSidebar: vi.fn().mockResolvedValue(undefined),
  hydrateWindowPaneLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/persistence/hydrate', () => ({
  hydratePreferences,
  hydrateSidebar,
  hydrateWindowPaneLayout,
}))

const {
  workspaceListFetch,
  projectDataFetch,
  setRepos,
  setProjects,
  workspaceListData,
  projectData,
} = vi.hoisted(() => ({
  workspaceListFetch: vi.fn().mockResolvedValue(undefined),
  projectDataFetch: vi.fn().mockResolvedValue(undefined),
  setRepos: vi.fn(),
  setProjects: vi.fn(),
  workspaceListData: { current: { status: 'idle' } as Loadable<Repo[]> },
  projectData: { current: { status: 'idle' } as Loadable<Project[]> },
}))
vi.mock('@/lib/store/workspace-list', () => ({
  useWorkspaceListStore: {
    getState: () => ({ fetch: workspaceListFetch, data: workspaceListData.current }),
  },
}))
vi.mock('@/lib/store/projects', () => ({
  useProjectDataStore: {
    getState: () => ({ fetch: projectDataFetch, data: projectData.current }),
  },
  useProjectStore: { getState: () => ({ setProjects }) },
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

const testProject: Project = {
  id: 'p1',
  name: 'project',
  path: '/tmp/p1',
  lastActivity: new Date('2026-01-01T00:00:00Z'),
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
    projectData.current = { status: 'idle' }
  })

  it('hydrates preferences and pane layout', async () => {
    await hydrateCriticalStores()

    expect(hydratePreferences).toHaveBeenCalledTimes(1)
    expect(hydrateWindowPaneLayout).toHaveBeenCalledTimes(1)
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

describe('hydrateProjectsInBackground', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    workspaceListData.current = { status: 'idle' }
    projectData.current = { status: 'idle' }
  })

  it('sets projects once the project-data fetch resolves, independently of the sidebar path', async () => {
    projectDataFetch.mockImplementation(async () => {
      projectData.current = { status: 'success', data: [testProject], fetchedAt: Date.now() }
    })

    hydrateProjectsInBackground()
    // Fire-and-forget by design — nothing to await from the caller's side,
    // just drain the microtask queue this test itself scheduled.
    await Promise.resolve()
    await Promise.resolve()

    expect(setProjects).toHaveBeenCalledWith([testProject])
    expect(setRepos).not.toHaveBeenCalled()
  })
})
