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
    await saveSidebarUI(['crowbar', 'quiver-core'])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['crowbar', 'quiver-core'])
  })

  it('overwrites previous record on second save', async () => {
    await saveSidebarUI(['crowbar'])
    await saveSidebarUI(['quiver-core'])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['quiver-core'])
  })
})
