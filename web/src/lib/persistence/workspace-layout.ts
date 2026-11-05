import { getDB } from './idb'
import type { WorkspaceLayout } from './schemas'

// Pane/buffer layout is window-level (panes hold chats from different workspaces), so there is one
// persisted layout per window; the store's `workspaceId` keyPath keys that one fixed row.
export const WINDOW_SESSION_ID = 'window'

/** 2: every chat pane records its workspace (`PaneGroup.workspaceId`).
 *  3: every editor buffer names its document's authority, a preview its source.
 *  4: editor buffers carry no text; unsaved text lives in `editor-drafts`. */
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
