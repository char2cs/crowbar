import { hydrateFromIDB } from '@/lib/persistence/hydrate'
import { getDB, resetDB } from '@/lib/persistence/idb'
import type { WorkspaceLayout, UIPreferences, EditorState } from '@/lib/persistence/schemas'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { IDBFactory } from 'fake-indexeddb'

async function seedDB(workspaceId: string) {
  const db = await getDB()
  const layout: WorkspaceLayout = {
    workspaceId,
    panes: [],
    activePane: 'pane-1',
    tabGroups: [],
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

describe('hydrateFromIDB', () => {
  beforeEach(async () => {
    // Reset the IDB singleton so each test gets a fresh database
    resetDB()
    // @ts-expect-error — reset global indexedDB to fresh instance
    globalThis.indexedDB = new IDBFactory()
  })

  afterEach(() => {
    destroyWorkspaceStore('missing-ws')
    destroyWorkspaceStore('ws-test')
  })

  it('returns null layout and prefs when IDB is empty', async () => {
    const result = await hydrateFromIDB('missing-ws')
    expect(result.layout).toBeNull()
    expect(result.prefs).toBeNull()
    expect(result.editorStates).toEqual([])
  })

  it('returns layout and prefs when seeded', async () => {
    const { layout, prefs, editorState } = await seedDB('ws-test')
    const result = await hydrateFromIDB('ws-test')
    expect(result.layout?.workspaceId).toBe(layout.workspaceId)
    expect(result.layout?.sidebarWidth).toBe(240)
    expect(result.prefs?.theme).toBe(prefs.theme)
    expect(result.editorStates).toHaveLength(1)
    expect(result.editorStates[0].bufferId).toBe('/src/main.ts')
  })
})
