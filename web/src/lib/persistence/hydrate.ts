import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useSettingsStore } from '@/features/settings/store'

export interface WorkspaceHydrationResult {
  layout: WorkspaceLayout | null
  editorStates: EditorState[]
}

export async function hydratePreferences(): Promise<UIPreferences | null> {
  const db = await getDB()
  const prefs = await db.get('ui-preferences', 'global').then(r => r ?? null)

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

  return prefs
}

export async function hydrateWorkspace(workspaceId: string): Promise<WorkspaceHydrationResult> {
  const db = await getDB()

  const [layout, editorStates] = await Promise.all([
    db.get('workspace-layout', workspaceId).then(r => r ?? null),
    db.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  if (layout) {
    const store = getOrCreateWorkspaceStore(workspaceId)
    store.setState({
      activePaneId: layout.activePaneId,
      mostRecentActivePaneIds: layout.mostRecentActivePaneIds ?? [layout.activePaneId],
      panes: layout.panes,
      rootLayout: layout.rootLayout,
      bottomLayout: layout.bottomLayout,
      buffers: layout.buffers ?? [],
    })
  }

  return { layout, editorStates }
}
