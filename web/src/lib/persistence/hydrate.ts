import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'

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

  return { layout, prefs, editorStates }
}
