import { saveSidebarUI, loadSidebarUI } from '@/lib/persistence/sidebar-ui'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('sidebar-ui persistence', () => {
  it('returns null when nothing is stored', async () => {
    const result = await loadSidebarUI()
    expect(result).toBeNull()
  })

  it('saves and loads collapsedRepos round-trip', async () => {
    await saveSidebarUI(['crowbar', 'quiver-core'], [])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['crowbar', 'quiver-core'])
  })

  it('saves and loads collapsedWorkspaces round-trip', async () => {
    await saveSidebarUI([], ['ws1', 'ws3'])
    const result = await loadSidebarUI()
    expect(result?.collapsedWorkspaces).toEqual(['ws1', 'ws3'])
  })

  it('overwrites previous record on second save', async () => {
    await saveSidebarUI(['crowbar'], [])
    await saveSidebarUI(['quiver-core'], ['ws1'])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['quiver-core'])
    expect(result?.collapsedWorkspaces).toEqual(['ws1'])
  })
})
