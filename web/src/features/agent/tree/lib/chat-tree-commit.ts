import {
  createChatFolder,
  setChatPlacement,
  updateChatFolder,
  type AgentChatFolder,
} from '@/features/agent/api/agent-api'
import {
  chatDropPlan,
  type ChatDragSubject,
  type ChatMove,
  type ResolvedChatDrop,
} from '@/features/agent/tree/lib/chat-drop'
import { toast } from '@/features/window/stores/toast-store'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Writing the Chats tree: a drop, a new folder, a rename.
 *
 * NOT a create. A chat is created WITH its parentage now (agent-api.ts), so
 * there is no "file this new chat into that row" step for this module to own —
 * the second write it used to be is exactly what let a thread's first turn run
 * before its lineage existed.
 *
 * DELETION is not here. It leaves through the removal tray, which holds the row
 * for eight seconds before anything is sent, so its whole path — plan, hide,
 * commit, snap back — lives in chat-removal.ts beside the pane gesture that
 * starts it.
 *
 * Every one of these is optimistic and then reconciled, in that order, because
 * the alternative is a tree that waits for a round trip before it agrees the
 * user moved anything. The pattern is the sidebar's: paint the whole level as it
 * will be, fire the calls in order, and put the level back exactly as it was if
 * the daemon refuses.
 *
 * IN ORDER is not a stylistic choice. Each `order` is a dense index into the
 * level as it stands once the previous call has landed, so firing a multi-row
 * move together lands it in an arbitrary arrangement.
 *
 * Kept out of the panel so the whole write path — including every refusal and
 * every snap-back — is exercisable without a synthetic pointer.
 */

type Snapshot = ReturnType<WorkspaceStore['getState']>

/** What a folder is called until the user says otherwise. */
export const NEW_FOLDER_NAME = 'New folder'

/**
 * Whether this id is a FOLDER. Everything else in the tree is a chat.
 *
 * Asked in that direction on purpose. The folder list is complete by
 * construction — a folder does not exist until its create has landed in the
 * store — while the chat list can legitimately be behind: a chat created a
 * moment ago is filed into its folder before the `created` frame that announces
 * it has arrived. Asking "is this a chat?" instead sent that chat's placement to
 * the FOLDER route, which is a PATCH on an aggregate that does not exist.
 */
function isFolder(st: Snapshot, id: string): boolean {
  return st.agentChats.folders.some((f) => f.id === id)
}

/**
 * Paint a level.
 *
 * A move is two writes on two arrays — the chats and the folders interleave at
 * every level and sort on one shared `order` — so this dispatches per row rather
 * than asking the caller to have split them.
 */
export function applyChatMoves(store: WorkspaceStore, moves: readonly ChatMove[]): void {
  const st = store.getState()
  const folders = new Map(st.agentChats.folders.map((f) => [f.id, f]))
  const next: AgentChatFolder[] = []
  for (const move of moves) {
    const folder = folders.get(move.id)
    if (folder) next.push({ ...folder, parentId: move.parentId, order: move.order })
    else st.setAgentChatPlacement(move.id, move.parentId, move.order)
  }
  if (next.length > 0) st.applyAgentChatFolders(next)
}

/** Where those rows sit RIGHT NOW — the undo for the paint above. */
export function captureChatMoves(
  store: WorkspaceStore,
  moves: readonly { id: string }[],
): ChatMove[] {
  const st = store.getState()
  const chats = new Map(st.agentChats.chats.map((c) => [c.id, c]))
  const folders = new Map(st.agentChats.folders.map((f) => [f.id, f]))
  const out: ChatMove[] = []
  for (const move of moves) {
    const row = chats.get(move.id) ?? folders.get(move.id)
    // A row that is not in the store cannot be restored, and inventing a
    // placement for it would be worse than leaving it alone.
    if (row) out.push({ id: move.id, parentId: row.parentId ?? '', order: row.order })
  }
  return out
}

/** One placement write, and the daemon's answer folded straight back in. */
async function sendMove(store: WorkspaceStore, wsId: string, move: ChatMove): Promise<void> {
  const st = store.getState()
  if (isFolder(st, move.id)) {
    const { folder, shifted } = await updateChatFolder(wsId, move.id, {
      parentId: move.parentId,
      order: move.order,
    })
    store.getState().applyAgentChatFolders([folder, ...shifted])
    return
  }
  const { chat, shifted } = await setChatPlacement(wsId, move.id, {
    parentId: move.parentId,
    order: move.order,
  })
  store.getState().upsertAgentChat(chat)
  // The rows the daemon's dense renumber moved that we did not ask about.
  // Dropping them leaves this client holding two rows that both think they are
  // third, until something happens to reseed the workspace.
  if (shifted.length > 0) store.getState().applyAgentChatFolders(shifted)
}

/**
 * Commit a drop.
 *
 * Resolves to nothing when the drop would change nothing — dropping a row onto
 * its own edge is the commonest accident in a list, and it must not become a
 * write the daemon has to serialise behind the ones that mean something.
 */
export async function commitChatDrop(
  store: WorkspaceStore,
  wsId: string,
  subjects: readonly ChatDragSubject[],
  target: ResolvedChatDrop,
  siblings: ReadonlyMap<string, readonly string[]>,
): Promise<void> {
  const plan = chatDropPlan(subjects, target, siblings)
  if (plan.calls.length === 0) return

  const undo = captureChatMoves(store, plan.writes)
  applyChatMoves(store, plan.writes)
  try {
    for (const call of plan.calls) {
      // react-doctor-disable-next-line async-await-in-loop -- deliberate: each `order` indexes the level as it stands once the previous call has landed, so firing these together files the rows in an arbitrary arrangement.
      await sendMove(store, wsId, call)
    }
  } catch (err) {
    // A refusal has to SAY something. The optimistic write means the row
    // visibly moves and then returns, which on its own reads as the drag having
    // missed — the user repeats the gesture and gets the same nothing.
    applyChatMoves(store, undo)
    toast.error(err instanceof Error ? err.message : 'That move was refused')
  }
}

/**
 * Make a folder inside `parentId` and hand back its id so the caller can open
 * the rename on it.
 *
 * Named for you rather than asked for up front: the gesture is "these belong
 * together", and the name is a detail you type into a row you can already see.
 * Returns '' when the daemon refused, so no rename editor opens on a row that
 * does not exist.
 */
export async function createChatFolderRow(
  store: WorkspaceStore,
  wsId: string,
  parentId: string,
): Promise<string> {
  try {
    const { folder, shifted } = await createChatFolder(wsId, NEW_FOLDER_NAME, parentId)
    store.getState().applyAgentChatFolders([folder, ...shifted])
    return folder.id
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to create the folder')
    return ''
  }
}

/**
 * Put rows in a new folder: make one where they already live, then file them
 * into it in order.
 *
 * The gesture is "these belong together", so the folder is a SIBLING of the rows
 * it collects rather than a thing at the root they would have to be dragged back
 * from. Its name is a detail the caller opens an editor on afterwards, over a
 * default — a modal asking for one before anything happens puts the naming
 * before the grouping, which is backwards.
 *
 * Sequential for the same reason every other multi-row write here is: each
 * `order` indexes the folder as it stands once the previous row has landed.
 *
 * Returns the new folder's id, or '' when nothing was made — a refused create,
 * or a selection with nothing in it — so no editor opens on a row that does not
 * exist.
 */
export async function groupIntoChatFolder(
  store: WorkspaceStore,
  wsId: string,
  subjects: readonly ChatDragSubject[],
): Promise<string> {
  if (subjects.length === 0) return ''
  // Where the rows already are. A mixed selection spanning two containers has no
  // one home, so the first row's is the one that wins — it is the row the user
  // right-clicked or collected first, and every alternative is a guess.
  const parentId = subjects[0].parentId ?? ''
  const id = await createChatFolderRow(store, wsId, parentId)
  if (!id) return ''

  const moves: ChatMove[] = subjects.map((s, order) => ({ id: s.id, parentId: id, order }))
  const undo = captureChatMoves(store, moves)
  applyChatMoves(store, moves)
  try {
    for (const move of moves) {
      // react-doctor-disable-next-line async-await-in-loop -- deliberate: each `order` indexes the folder as it stands once the previous row has landed, so firing these together files them in an arbitrary arrangement.
      await sendMove(store, wsId, move)
    }
  } catch (err) {
    // The folder is left behind on purpose: it exists on the server now, and
    // silently deleting it would be a second write racing the one that just
    // failed. The rows go back where they were.
    applyChatMoves(store, undo)
    toast.error(err instanceof Error ? err.message : 'Those rows could not be filed')
  }
  return id
}

/** Rename a folder, optimistically. */
export async function renameChatFolderRow(
  store: WorkspaceStore,
  wsId: string,
  folderId: string,
  name: string,
): Promise<void> {
  const previous = store.getState().agentChats.folders.find((f) => f.id === folderId)
  if (!previous || previous.name === name) return
  store.getState().applyAgentChatFolders([{ ...previous, name }])
  try {
    const { folder, shifted } = await updateChatFolder(wsId, folderId, { name })
    store.getState().applyAgentChatFolders([folder, ...shifted])
  } catch (err) {
    store.getState().applyAgentChatFolders([previous])
    toast.error(err instanceof Error ? err.message : 'Failed to rename the folder')
  }
}
