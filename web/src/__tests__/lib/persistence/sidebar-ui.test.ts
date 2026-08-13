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
    await saveSidebarUI({ collapsedRepos: ['crowbar', 'quiver-core'], collapsedWorkspaces: [] })
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['crowbar', 'quiver-core'])
  })

  it('saves and loads collapsedWorkspaces round-trip', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedWorkspaces: ['ws1', 'ws3'] })
    const result = await loadSidebarUI()
    expect(result?.collapsedWorkspaces).toEqual(['ws1', 'ws3'])
  })

  it('saves and loads collapsedChatRows round-trip', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedChatRows: ['folder-1', 'chat-9'] })
    const result = await loadSidebarUI()
    expect(result?.collapsedChatRows).toEqual(['folder-1', 'chat-9'])
  })

  it('overwrites previous record on second save', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    await saveSidebarUI({ collapsedRepos: ['quiver-core'], collapsedWorkspaces: ['ws1'] })
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['quiver-core'])
    expect(result?.collapsedWorkspaces).toEqual(['ws1'])
  })

  // A record written before the Chats panel was collapsible has no such key.
  // Pre-production the fallback IS the migration: it must read as "nothing
  // folded" rather than crash the hydrate that overlays it.
  it('loads a record that predates collapsedChatRows', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    const result = await loadSidebarUI()
    expect(result?.collapsedChatRows).toBeUndefined()
  })
})
