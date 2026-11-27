import { getDB } from './idb'
import type { WorkspaceLayout } from './schemas'

const WINDOW_SESSION_ID = 'window'

export const WINDOW_LAYOUT_VERSION = 4

export async function saveWorkspaceLayout(layout: WorkspaceLayout): Promise<void> {
  const db = await getDB()
  await db.put('workspace-layout', {
    ...layout,
    workspaceId: WINDOW_SESSION_ID,
    version: WINDOW_LAYOUT_VERSION,
    updatedAt: Date.now(),
  })
}

export async function loadWindowPaneLayout(): Promise<WorkspaceLayout | null> {
  const db = await getDB()
  return (await db.get('workspace-layout', WINDOW_SESSION_ID)) ?? null
}
