import { saveWorkspaceLayout, WINDOW_SESSION_ID } from '@/lib/persistence/workspace-layout'
import { loadWindowPaneLayout } from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createLeaf } from '@/features/panes/utils/pane-layout'

const mockLayout: WorkspaceLayout = {
  // Task 26: pane/buffer layout is window-level now — the object store's
  // keyPath is still literally `workspaceId`, but the value written there is
  // always the fixed WINDOW_SESSION_ID (saveWorkspaceLayout stamps it).
  workspaceId: WINDOW_SESSION_ID,
  panes: {
    [ROOT_PANE_ID]: {
      id: ROOT_PANE_ID,
      type: 'group',
      chatId: null,
      runnerId: null,
      editorTabIds: [],
      activeEditorTabId: null,
      editorOpen: false,
    },
  },
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

describe('window pane layout persistence', () => {
  it('saves and loads layout round-trip', async () => {
    await saveWorkspaceLayout(mockLayout)
    const loaded = await loadWindowPaneLayout()
    expect(loaded?.activePaneId).toBe(ROOT_PANE_ID)
    expect(loaded?.sidebarWidth).toBe(260)
  })

  it('returns null when nothing has been saved yet', async () => {
    const loaded = await loadWindowPaneLayout()
    expect(loaded).toBeNull()
  })
})
