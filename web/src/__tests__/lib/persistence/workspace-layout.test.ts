import { saveWorkspaceLayout, loadWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createLeaf } from '@/features/panes/utils/pane-layout'

const mockLayout: WorkspaceLayout = {
  workspaceId: 'ws-persist-test',
  panes: { [ROOT_PANE_ID]: { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null } },
  rootLayout: createLeaf(ROOT_PANE_ID),
  bottomLayout: createLeaf('bottom-pane'),
  activePaneId: ROOT_PANE_ID,
  mostRecentActivePaneIds: [ROOT_PANE_ID],
  buffers: [],
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
    expect(loaded?.activePaneId).toBe(ROOT_PANE_ID)
    expect(loaded?.sidebarWidth).toBe(260)
  })

  it('returns null for unknown workspaceId', async () => {
    const loaded = await loadWorkspaceLayout('nonexistent')
    expect(loaded).toBeNull()
  })
})
