import { reparentWorkspace, handleWorkspaceReparented } from '@/lib/api/workspace'
import { loadWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

const WORKSPACE_TEST_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'ws3', branch: 'feature/app-design', parentId: 'ws-develop', status: 'pr-open', age: '16h ago' },
    ],
  },
]

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useSidebarStore.setState({
    chats: [],
    repos: WORKSPACE_TEST_REPOS.map(r => ({ ...r, workspaces: [...r.workspaces] })),
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    activeTab: 'workspaces',
  })
})

describe('handleWorkspaceReparented', () => {
  it('updates the sidebar store parentId', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    const ws = repo.workspaces.find(w => w.id === 'ws3')!
    expect(ws.parentId).toBe('ws-develop')
  })

  it('writes hierarchy to IDB after updating store', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy).not.toBeNull()
    const entry = hierarchy!.entries.find(e => e.wsId === 'ws3')
    expect(entry?.parentId).toBe('ws-develop')
  })

  it('removes parentId when newParentId is undefined', async () => {
    await handleWorkspaceReparented('ws3', undefined, 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    const ws = repo.workspaces.find(w => w.id === 'ws3')!
    expect(ws.parentId).toBeUndefined()
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    const entry = hierarchy!.entries.find(e => e.wsId === 'ws3')
    expect(entry?.parentId).toBeUndefined()
  })
})

describe('reparentWorkspace', () => {
  it('resolves without throwing', async () => {
    await expect(reparentWorkspace('ws3', 'ws-develop', 'crowbar')).resolves.toBeUndefined()
  })

  it('updates store and IDB (delegates to handler)', async () => {
    await reparentWorkspace('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    expect(repo.workspaces.find(w => w.id === 'ws3')?.parentId).toBe('ws-develop')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy?.entries.find(e => e.wsId === 'ws3')?.parentId).toBe('ws-develop')
  })
})
