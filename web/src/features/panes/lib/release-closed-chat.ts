import type { PaneGroup } from '@/features/panes/types/pane'
import { stopChat } from '@/features/agent/api/agent-api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { requestWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'

/** Read the window's panes as they stand RIGHT NOW. Passed in rather than
 *  imported so this module never reaches back into `window-pane-store`, whose
 *  own `pane-slice` calls it — and so the post-`stopChat` re-read below is a
 *  genuinely fresh read rather than a stale snapshot taken before the await. */
export type ReadPanes = () => Record<string, PaneGroup>

function chatIsUp(readPanes: ReadPanes, chatId: string): boolean {
  return Object.values(readPanes()).some((pane) => pane.chatId === chatId)
}

/**
 * A pane holding `chatId` has CLOSED — tear the chat down on both sides.
 *
 * "All of Crowbar's chats should die once the user has closed their view...
 * Both. It's like killing a chat tab: removes both out of memory."
 *
 *  - **Backend**: `stopChat` — the exact call its own doc says closing a chat
 *    tab makes. The vendor CLI stops; the chat entry and its bound
 *    conversation are KEPT, so the row stays in the tree and reopening it
 *    later revives the real conversation through the normal resume path.
 *    Deliberately NOT `deleteChat`: closing a view ends the view, never the
 *    chat.
 *  - **Frontend**: once the backend has settled, the owning workspace's live
 *    store is dropped — but only if that workspace has nothing else on
 *    screen. A workspace owns many chats, and closing one view of it says
 *    nothing about the others.
 *
 * The two are SEQUENCED, not raced. `stopChat` is what actually stops a chat
 * that is mid-turn, and the store being torn down is the same store that
 * chat's stream, working map and message log live on — dropping it first
 * would yank an in-flight turn's own bookkeeping out from under it while the
 * CLI is still writing. So the stop is awaited, and every "is this still
 * wanted" question is re-asked afterwards against fresh state: a close is
 * undoable (Recents remembers it) and the user can perfectly well reopen the
 * chat while the stop is in flight.
 */
export async function releaseClosedChat(
  chatId: string,
  workspaceId: string | null,
  readPanes: ReadPanes,
): Promise<void> {
  // The same chat can be up in more than one pane (a drag onto a second pane
  // reveals rather than duplicates, but a persisted layout can still carry
  // two). Closing one of them ends that VIEW, not the chat.
  if (chatIsUp(readPanes, chatId)) return
  // The workspace was recorded on the pane when the chat was opened (C3); a
  // pane from before that has nothing to address the stop to.
  if (!workspaceId) return

  // A stop that FAILED means the CLI is still running — the chat is live, its
  // stream is still writing into this workspace's store, and evicting that
  // store would orphan a running turn with nothing left watching it. Give up
  // on the frontend half rather than tear down over a chat we did not manage
  // to stop; ordinary keep-alive retention still ages the workspace out.
  try {
    await stopChat(workspaceId, chatId)
  } catch (err) {
    if (import.meta.env.DEV) console.warn('stop chat for closed view failed:', err)
    return
  }

  // Re-read: the close is undoable, and clicking the row again while the stop
  // was in flight puts the chat straight back on screen. Reviving it is the
  // resume path's job; what matters here is not destroying the store under it.
  if (chatIsUp(readPanes, chatId)) return

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
