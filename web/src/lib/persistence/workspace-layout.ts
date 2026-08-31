import { getDB } from './idb'
import type { WorkspaceLayout } from './schemas'

/**
 * Task 26: pane/buffer layout is window-level, not per-workspace (panes can
 * now hold chats from different workspaces in the same tree — see
 * window-pane-store.ts), so there is exactly one persisted layout per
 * window/session rather than one per workspace id. The object store's
 * `keyPath` (`workspaceId`, unchanged from before this task) is reused as
 * the key for this one fixed row.
 */
export const WINDOW_SESSION_ID = 'window'

export async function saveWorkspaceLayout(layout: WorkspaceLayout): Promise<void> {
  const db = await getDB()
  await db.put('workspace-layout', { ...layout, workspaceId: WINDOW_SESSION_ID, updatedAt: Date.now() })
}

export async function loadWindowPaneLayout(): Promise<WorkspaceLayout | null> {
  const db = await getDB()
  return (await db.get('workspace-layout', WINDOW_SESSION_ID)) ?? null
}
