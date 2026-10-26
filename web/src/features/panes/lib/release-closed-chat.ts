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

/**
 * A pane holding `chatId` has CLOSED. That detaches this window from the chat
 * and nothing more: the agent keeps running (only an explicit Kill ends it), so
 * the one thing left to release is the browser's copy.
 *
 * The owning workspace's live store is dropped, but only if that workspace has
 * nothing else on screen. A workspace owns many chats, and closing one view of
 * it says nothing about the others.
 */
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

  // The ACTIVE workspace is the route: `WorkspaceView` is mounted over its
  // store and would re-create it the instant it went away, so there is
  // nothing to gain and a live subtree to break. It stops being active the
  // moment the user goes anywhere else, and ordinary retention takes it from
  // there.
  if (workspaceId === getActiveWorkspaceId()) return

  requestWorkspaceEviction(workspaceId)
}
