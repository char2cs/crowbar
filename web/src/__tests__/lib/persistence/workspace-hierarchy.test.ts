import {
  saveWorkspaceHierarchy,
  loadWorkspaceHierarchy,
  loadAllWorkspaceHierarchies,
} from '@/lib/persistence/workspace-hierarchy'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('workspace-hierarchy persistence', () => {
  it('returns null for unknown repoId', async () => {
    const result = await loadWorkspaceHierarchy('nope')
    expect(result).toBeNull()
  })

  it('saves and loads entries round-trip', async () => {
    const entries = [
      { wsId: 'ws1', parentId: 'ws-develop' },
      { wsId: 'ws2' },
    ]
    await saveWorkspaceHierarchy('crowbar', entries)
    const result = await loadWorkspaceHierarchy('crowbar')
    expect(result?.repoId).toBe('crowbar')
    expect(result?.entries).toEqual(entries)
  })

  it('overwrites previous record for same repoId', async () => {
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1', parentId: 'ws-develop' }])
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1' }])
    const result = await loadWorkspaceHierarchy('crowbar')
    expect(result?.entries[0].parentId).toBeUndefined()
  })

  it('loadAllWorkspaceHierarchies returns all stored records', async () => {
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1' }])
    await saveWorkspaceHierarchy('quiver-core', [{ wsId: 'qc1', parentId: 'qc-develop' }])
    const all = await loadAllWorkspaceHierarchies()
    expect(all).toHaveLength(2)
    const repoIds = all.map(h => h.repoId)
    expect(repoIds).toContain('crowbar')
    expect(repoIds).toContain('quiver-core')
  })

  it('loadAllWorkspaceHierarchies returns empty array when nothing stored', async () => {
    const all = await loadAllWorkspaceHierarchies()
    expect(all).toEqual([])
  })
})
