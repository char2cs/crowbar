import { saveSidebarUI, loadSidebarUI } from '@/lib/persistence/sidebar-ui'
import { IDBFactory } from 'fake-indexeddb'

beforeEach(() => {
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

  it('saves and loads the sidebar mode round-trip', async () => {
    await saveSidebarUI({ collapsedChatRows: [], activeTab: 'git' })
    const result = await loadSidebarUI()
    expect(result?.activeTab).toBe('git')
  })

  it('saves and loads the host filter round-trip', async () => {
    await saveSidebarUI({ collapsedChatRows: [], hostFilter: 'h1' })
    const result = await loadSidebarUI()
    expect(result?.hostFilter).toBe('h1')
  })

  it('overwrites previous record on second save', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    await saveSidebarUI({ collapsedRepos: ['quiver-core'], collapsedWorkspaces: ['ws1'] })
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['quiver-core'])
    expect(result?.collapsedWorkspaces).toEqual(['ws1'])
  })

  it('loads a record that predates collapsedChatRows', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    const result = await loadSidebarUI()
    expect(result?.collapsedChatRows).toBeUndefined()
  })
})
