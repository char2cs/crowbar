import type { PaneGroup } from '@/features/panes/types/pane'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { requestWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'

/** Read the window's panes as they stand RIGHT NOW. Passed in rather than
 *  imported so this module never reaches back into `window-pane-store`, whose
 *  own `pane-slice` calls it. */
export type ReadPanes = () => Record<string, PaneGroup>

function chatIsUp(readPanes: ReadPanes, chatId: string): boolean {
  return Object.values(readPanes()).some((pane) => pane.chatId === chatId)
}

/** A pane holding `chatId` closed. The agent keeps running (only Kill ends it); all that is
 *  released is the browser's copy — the workspace's live store, only when none of its other
 *  chats is on screen. */
export function releaseClosedChat(
  chatId: string,
  workspaceId: string | null,
  readPanes: ReadPanes,
): void {
  // The same chat can be up in more than one pane (a drag onto a second pane
  // reveals rather than duplicates, but a persisted layout can still carry
  // two). Closing one of them ends that VIEW, not the chat.
  if (chatIsUp(readPanes, chatId)) return
  // The workspace was recorded on the pane when the chat was opened (C3).
  if (!workspaceId) return

  // The workspace still has a view of its own up — some other chat OF ITS OWN
  // is on screen — so it is in use, not closed.
  for (const pane of Object.values(readPanes())) {
    if (pane.chatId && pane.workspaceId === workspaceId) return
  }

  // The active workspace is the route: WorkspaceView would re-create its store at once. Ordinary
  // retention takes it once the user navigates away.
  if (workspaceId === getActiveWorkspaceId()) return

  requestWorkspaceEviction(workspaceId)
}
