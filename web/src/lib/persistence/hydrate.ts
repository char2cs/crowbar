import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useSettingsStore } from '@/features/settings/store'

export interface HydrationResult {
  layout: WorkspaceLayout | null
  prefs: UIPreferences | null
  editorStates: EditorState[]
}

export async function hydrateFromIDB(workspaceId: string): Promise<HydrationResult> {
  const db = await getDB()

  const [layout, prefs, editorStates] = await Promise.all([
    db.get('workspace-layout', workspaceId).then(r => r ?? null),
    db.get('ui-preferences', 'global').then(r => r ?? null),
    db.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  if (layout) {
    const store = getOrCreateWorkspaceStore(workspaceId)
    store.setState({
      paneRoot: layout.panes[0] as ReturnType<typeof store.getState>['paneRoot'] | undefined
        ?? store.getState().paneRoot,
      bottomRoot: layout.tabGroups[0] as ReturnType<typeof store.getState>['bottomRoot'] | undefined
        ?? store.getState().bottomRoot,
      activePaneId: layout.activePane,
    })
  }

  if (prefs) {
    useSettingsStore.setState((state) => ({
      settings: {
        ...state.settings,
        theme: prefs.theme,
        fontSize: prefs.fontSize,
        fontFamily: prefs.fontFamily,
        tabSize: prefs.tabSize,
        wordWrap: prefs.wordWrap,
        showMinimap: prefs.minimap,
      },
    }))
  }

  return { layout, prefs, editorStates }
}
