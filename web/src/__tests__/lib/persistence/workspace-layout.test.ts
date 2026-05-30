import { saveWorkspaceLayout, loadWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

const mockLayout: WorkspaceLayout = {
  workspaceId: 'ws-persist-test',
  panes: [],
  activePane: 'pane-a',
  tabGroups: [],
  sidebarWidth: 260,
  rightSidebarWidth: 300,
  updatedAt: 1000,
}

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('workspace layout persistence', () => {
  it('saves and loads layout round-trip', async () => {
    await saveWorkspaceLayout(mockLayout)
    const loaded = await loadWorkspaceLayout('ws-persist-test')
    expect(loaded?.activePane).toBe('pane-a')
    expect(loaded?.sidebarWidth).toBe(260)
  })

  it('returns null for unknown workspaceId', async () => {
    const loaded = await loadWorkspaceLayout('nonexistent')
    expect(loaded).toBeNull()
  })
})
