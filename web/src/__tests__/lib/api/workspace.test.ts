import { reparentWorkspace, handleWorkspaceReparented } from '@/lib/api/workspace'
import { loadWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

const WORKSPACE_TEST_REPOS: Repo[] = [
  {
    id: 'crowbar',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      {
        id: 'ws3',
        branch: 'feature/app-design',
        parentId: 'ws-develop',
        status: 'pr-open',
        age: '16h ago',
      },
    ],
  },
]

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useSidebarStore.setState({
    chats: [],
    repos: WORKSPACE_TEST_REPOS.map((r) => ({ ...r, workspaces: [...r.workspaces] })),
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    activeTab: 'workspaces',
  })
})

describe('handleWorkspaceReparented', () => {
  it('updates the sidebar store parentId', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    const ws = repo.workspaces.find((w) => w.id === 'ws3')!
    expect(ws.parentId).toBe('ws-develop')
  })

  it('writes hierarchy to IDB after updating store', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy).not.toBeNull()
    const entry = hierarchy!.entries.find((e) => e.wsId === 'ws3')
    expect(entry?.parentId).toBe('ws-develop')
  })

  it('removes parentId when newParentId is undefined', async () => {
    await handleWorkspaceReparented('ws3', undefined, 'crowbar')
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    const ws = repo.workspaces.find((w) => w.id === 'ws3')!
    expect(ws.parentId).toBeUndefined()
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    const entry = hierarchy!.entries.find((e) => e.wsId === 'ws3')
    expect(entry?.parentId).toBeUndefined()
  })
})

describe('reparentWorkspace', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: { id: 'ws3' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('POSTs to /v0/workspaces/:wsId/reparent with the new parent id', async () => {
    await reparentWorkspace('ws3', 'ws-develop', 'crowbar')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/workspaces/ws3/reparent')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ newParentId: 'ws-develop' })
  })

  it('updates store and IDB after a successful backend call', async () => {
    await reparentWorkspace('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    expect(repo.workspaces.find((w) => w.id === 'ws3')?.parentId).toBe('ws-develop')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy?.entries.find((e) => e.wsId === 'ws3')?.parentId).toBe('ws-develop')
  })

  it('does not touch the local store when the backend call fails', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: false, error: 'has children' }), {
        status: 409,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await expect(reparentWorkspace('ws3', 'ws-other', 'crowbar')).rejects.toThrow('has children')
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    // ws3 keeps its original parent — the local handler must not run on failure.
    expect(repo.workspaces.find((w) => w.id === 'ws3')?.parentId).toBe('ws-develop')
  })

  it('skips the backend call when moving to the repo root (newParentId undefined)', async () => {
    await reparentWorkspace('ws3', undefined, 'crowbar')
    expect(fetchMock).not.toHaveBeenCalled()
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    expect(repo.workspaces.find((w) => w.id === 'ws3')?.parentId).toBeUndefined()
  })
})
