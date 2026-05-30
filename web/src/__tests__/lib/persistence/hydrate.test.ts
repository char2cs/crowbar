import { hydrateFromIDB } from '@/lib/persistence/hydrate'
import { getDB } from '@/lib/persistence/idb'
import type { WorkspaceLayout, UIPreferences } from '@/lib/persistence/schemas'

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
  await db.put('workspace-layout', layout)
  await db.put('ui-preferences', prefs, 'global')
  return { layout, prefs }
}

describe('hydrateFromIDB', () => {
  it('returns null layout and prefs when IDB is empty', async () => {
    const result = await hydrateFromIDB('missing-ws')
    expect(result.layout).toBeNull()
    expect(result.prefs).toBeNull()
    expect(result.editorStates).toEqual([])
  })

  it('returns layout and prefs when seeded', async () => {
    const { layout, prefs } = await seedDB('ws-test')
    const result = await hydrateFromIDB('ws-test')
    expect(result.layout?.workspaceId).toBe(layout.workspaceId)
    expect(result.layout?.sidebarWidth).toBe(240)
    expect(result.prefs?.theme).toBe(prefs.theme)
  })
})
