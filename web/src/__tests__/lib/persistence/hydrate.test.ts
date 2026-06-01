import { hydrateWorkspace, hydratePreferences } from '@/lib/persistence/hydrate'
import { getDB, resetDB } from '@/lib/persistence/idb'
import type { WorkspaceLayout, UIPreferences, EditorState } from '@/lib/persistence/schemas'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { IDBFactory } from 'fake-indexeddb'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createLeaf } from '@/features/panes/utils/pane-layout'

async function seedDB(workspaceId: string) {
  const db = await getDB()
  const layout: WorkspaceLayout = {
    workspaceId,
    panes: { [ROOT_PANE_ID]: { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null } },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf('bottom-pane'),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    buffers: [],
    sidebarWidth: 240,
    rightSidebarWidth: 280,
    updatedAt: Date.now(),
  }
  const prefs: UIPreferences = {
    theme: 'dark',
    fontSize: 14,
    fontFamily: 'JetBrains Mono',
    tabSize: 2,
    wordWrap: false,
    minimap: true,
    updatedAt: Date.now(),
  }
  const editorState: EditorState = {
    workspaceId,
    bufferId: '/src/main.ts',
    cursorLine: 10,
    cursorColumn: 5,
    scrollTop: 200,
    folds: [],
    updatedAt: Date.now(),
  }
  await db.put('workspace-layout', layout)
  await db.put('ui-preferences', prefs, 'global')
  await db.put('editor-state', editorState)
  return { layout, prefs, editorState }
}

describe('hydrateWorkspace', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
  })

  afterEach(() => {
    destroyWorkspaceStore('missing-ws')
    destroyWorkspaceStore('ws-test')
  })

  it('returns null layout and empty editor states when IDB is empty', async () => {
    const result = await hydrateWorkspace('missing-ws')
    expect(result.layout).toBeNull()
    expect(result.editorStates).toEqual([])
  })

  it('returns layout and editor states when seeded', async () => {
    const { layout, editorState } = await seedDB('ws-test')
    const result = await hydrateWorkspace('ws-test')
    expect(result.layout?.workspaceId).toBe(layout.workspaceId)
    expect(result.layout?.sidebarWidth).toBe(240)
    expect(result.editorStates).toHaveLength(1)
    expect(result.editorStates[0].bufferId).toBe(editorState.bufferId)
  })
})

describe('hydratePreferences', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
  })

  it('returns null when no prefs are stored', async () => {
    const prefs = await hydratePreferences()
    expect(prefs).toBeNull()
  })

  it('returns stored preferences', async () => {
    const { prefs } = await seedDB('ws-test')
    const result = await hydratePreferences()
    expect(result?.theme).toBe(prefs.theme)
  })
})
