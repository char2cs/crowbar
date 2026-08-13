import { deleteChat, deleteChatFolder } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { applyChatMoves, captureChatMoves } from './chat-tree-commit'
import { chatSubtreeIds, type ChatLike, type FolderLike } from './chat-rows'
import type { ChatDragSubject } from './chat-drop'
import type { RemovalOverlayText } from '@/components/layout/editor-removal-overlay'
import type { RemovalDraft, RemovalEntry } from '@/lib/store/sidebar-removal'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Removing rows from the Chats tree — planned, described, and finally sent.
 *
 * The gesture is the SIDEBAR'S, and deliberately not a second one: a row is
 * carried onto the editor pane, the pane veils itself and names what would go,
 * and a release drops the row into the removal tray for eight seconds before
 * anything is destroyed. Every part of that is shared (drop-target-dom.ts's
 * pane attribute, editor-removal-overlay.tsx's two states, sidebar-removal.ts's
 * tray). What is HERE is only the chats half: which rows go WITH a row, what the
 * veil says about that, and what the delete actually is.
 *
 * The one fact this file exists to tell the truth about is that the two kinds
 * differ:
 *
 *   - A CHAT cascades. Its threads read its turns, so a thread left behind is a
 *     conversation reading a context that no longer exists. They go with it.
 *   - A FOLDER promotes. It holds no turns, so the chats inside it outlive it
 *     and move up to where it sat — exactly as the workspace tree's folders do.
 *
 * Both statements have to survive into what the pane's veil says and into what
 * the panel draws while a row is held, or the undo window is offering to undo
 * something other than what was shown.
 *
 * Pure up to {@link sendChatRemoval}, which is the single step that destroys
 * anything — the same split `removal-plan.ts` / `removal-commit.ts` keeps next
 * door.
 */

/** What the tray gives you back, said the same way the sidebar says it. */
const UNDO_DETAIL = 'You will have 8 seconds to undo'

/** Whether this row left through the Chats tree rather than the sidebar. */
export function isChatRemoval(entry: { kind: RemovalDraft['kind'] }): boolean {
  return entry.kind === 'chat' || entry.kind === 'chatFolder'
}

export interface ChatRemovalInput {
  /** The workspace whose chats these are — what the DELETE is addressed to. */
  wsId: string
  chats: readonly ChatLike[]
  folders: readonly FolderLike[]
  /**
   * providerId → its artwork, the SAME map the rows draw their glyphs from.
   *
   * Taken in rather than re-derived here so a held row cannot end up wearing a
   * different mark from the row it came from — which is exactly the defect the
   * tray had when it drew its own stand-in.
   */
  providerIcons: ReadonlyMap<string, string>
  /** parentId → children, from the same build the rows on screen came from. */
  siblings: ReadonlyMap<string, readonly string[]>
}

/**
 * The rows a removal hides, and the rows that go with them.
 *
 * `hiddenIds` runs DEEPEST FIRST with the row itself last, because that is the
 * order the delete has to go out in: a parent deleted before its child leaves
 * the child pointing at nothing for as long as the requests are in flight.
 *
 * A row already inside another subject's subtree is not a second removal — it is
 * part of the first — so it is claimed and skipped, or the tray would hold two
 * rows for one disappearance.
 */
export function planChatRemoval(
  subjects: readonly ChatDragSubject[],
  input: ChatRemovalInput,
): RemovalDraft[] {
  const chats = new Map(input.chats.map((c) => [c.id, c]))
  const names = new Map(input.folders.map((f) => [f.id, f.name]))

  const drafts: RemovalDraft[] = []
  const claimed = new Set<string>()

  for (const subject of subjects) {
    if (claimed.has(subject.id)) continue
    const chat = chats.get(subject.id)
    const label = subject.kind === 'chat' ? chat?.title : names.get(subject.id)
    // A row the store no longer holds cannot be removed, and inventing a label
    // for it would put a row in the tray naming nothing.
    if (label === undefined) continue

    // A folder takes only itself: its contents are promoted, not deleted.
    const hiddenIds =
      subject.kind === 'chat'
        ? [...chatSubtreeIds(input.siblings, subject.id), subject.id]
        : [subject.id]

    drafts.push({
      kind: subject.kind,
      id: subject.id,
      label,
      wsId: input.wsId,
      // The chat's OWN mark, so a held row looks like the row it came from. A
      // provider that has gone leaves '', and the tray draws the same chat-glyph
      // fallback the sidebar row draws.
      providerIcon: chat ? (input.providerIcons.get(chat.activeProviderId) ?? '') : '',
      // A chat belongs to a workspace, not to a project or a repo — the two
      // fields the sidebar's rows are addressed by mean nothing here.
      projectId: '',
      repoId: '',
      hiddenIds,
      extra: hiddenIds.length - 1,
      // Nothing to navigate to: removing a chat never removes the workspace the
      // editor is showing.
      fallbackWsId: null,
    })
    for (const id of hiddenIds) claimed.add(id)
  }

  return drafts
}

/**
 * What the pane's veil says while a chat row is over it.
 *
 * It names the row, because the pane is a large target and the panel behind it
 * may be scrolled somewhere else by the time the pointer arrives. And it says
 * which of the two rules applies, because "release to remove" over a chat with
 * three threads under it and over an empty folder promise very different things.
 */
export function describeChatRemoval(
  drafts: readonly RemovalDraft[],
  armed = true,
): RemovalOverlayText {
  // "Drop here" while the zone is merely AVAILABLE, "Release" only once a
  // release really would remove — the pane is up for the whole drag, and for
  // most of it the pointer is somewhere else entirely.
  const verb = armed ? 'Release to remove' : 'Drop here to remove'

  if (drafts.length === 1) {
    const [only] = drafts
    return { title: `${verb} ${only.label}`, detail: detailFor(only), armed }
  }
  return { title: `${verb} ${drafts.length} rows`, detail: UNDO_DETAIL, armed }
}

function detailFor(draft: RemovalDraft): string {
  // A folder is a way of LOOKING at chats; the chats outlive it. Said outright,
  // because a veil that only counted down would leave the user to guess whether
  // the conversations inside are going with it.
  if (draft.kind === 'chatFolder') return `Its contents move up one level · ${UNDO_DETAIL}`
  if (draft.extra === 0) return UNDO_DETAIL
  // Rows, not "threads": a chat's subtree holds folders as well, and they go
  // with it too. Counting them as threads would overstate what is being lost.
  const going = draft.extra === 1 ? '1 nested row goes' : `${draft.extra} nested rows go`
  return `${going} with it · ${UNDO_DETAIL}`
}

/**
 * The tree as it reads with the tray's rows taken out.
 *
 * A held row is HIDDEN, never deleted, so this is a projection over what the
 * store already has — which is what makes Cancel a matter of dropping an id
 * rather than putting a subtree back together.
 *
 * A held FOLDER is the one case that rewrites rather than filters: deleting one
 * promotes its children to the folder's own parent, so the preview has to show
 * them there. Filtering the folder alone would re-root them (the tree grounds a
 * row whose parent names nothing), which is a different place and a promise the
 * commit would not keep.
 *
 * Returns the very arrays it was given when nothing is held, so the panel's
 * memo on the tree build is untouched by a store that has no removals in it.
 */
export function applyPendingChatRemovals<C extends ChatLike, F extends FolderLike>(
  chats: readonly C[],
  folders: readonly F[],
  hiddenIds: ReadonlySet<string>,
): { chats: readonly C[]; folders: readonly F[] } {
  if (hiddenIds.size === 0) return { chats, folders }

  const byId = new Map(folders.map((f) => [f.id, f]))

  // Walk each held folder's own parent up past any held ancestor, so a hold that
  // takes a folder and the folder around it still lands the survivors at the
  // outermost place that is actually still on screen.
  const survivingParentOf = (folderId: string): string => {
    let cursor = byId.get(folderId)?.parentId ?? ''
    const seen = new Set<string>([folderId])
    while (cursor && hiddenIds.has(cursor) && !seen.has(cursor)) {
      seen.add(cursor)
      cursor = byId.get(cursor)?.parentId ?? ''
    }
    return cursor
  }

  const rehomed = new Map<string, string>()
  for (const folder of folders) {
    if (hiddenIds.has(folder.id)) rehomed.set(folder.id, survivingParentOf(folder.id))
  }

  const rehome = <R extends { parentId?: string }>(row: R): R => {
    const parentId = row.parentId ?? ''
    return rehomed.has(parentId) ? { ...row, parentId: rehomed.get(parentId) } : row
  }

  // One pass per array rather than filter-then-map: this sits inside the panel's
  // tree memo, so it is on the path of every keystroke and every drop for as long
  // as the tray is holding anything.
  const surviving = <R extends { id: string; parentId?: string }>(rows: readonly R[]): R[] => {
    const out: R[] = []
    for (const row of rows) {
      if (!hiddenIds.has(row.id)) out.push(rehome(row))
    }
    return out
  }

  return { chats: surviving(chats), folders: surviving(folders) }
}

/**
 * Fire the removal a tray entry has been holding — the one step here that
 * destroys anything.
 *
 * The store is emptied FIRST and the requests go out afterwards, which is what
 * lets the tray stop hiding the ids the moment this resolves: the rows are
 * already gone from the panel, so there is nothing left to flash back while a
 * tombstone finds its way over the wire.
 *
 * Throws on refusal, with the store already put back. The tray's commit path
 * turns that into the toast and the un-hiding — there is nobody at the gesture
 * any more to tell, which is exactly why this is the only path that can surface
 * a failure at all.
 */
export function sendChatRemoval(entry: RemovalEntry, init?: RequestInit): Promise<void> {
  const store = getOrCreateWorkspaceStore(entry.wsId)
  return entry.kind === 'chatFolder'
    ? sendFolderRemoval(store, entry, init)
    : sendSubtreeRemoval(store, entry, init)
}

/** A folder: gone, its children promoted to where it sat. */
async function sendFolderRemoval(
  store: WorkspaceStore,
  entry: RemovalEntry,
  init?: RequestInit,
): Promise<void> {
  const before = store.getState()
  const previous = before.agentChats.folders.find((f) => f.id === entry.id)
  if (!previous) return
  // The children move the moment the folder does, so the undo is the folder AND
  // everything it was holding. Restoring the row alone would leave its contents
  // scattered at the grandparent with the folder back around nothing.
  const orphans = captureChatMoves(store, [
    ...before.agentChats.chats.filter((c) => (c.parentId ?? '') === entry.id),
    ...before.agentChats.folders.filter((f) => (f.parentId ?? '') === entry.id),
  ])
  store.getState().removeAgentChatFolder(entry.id)
  try {
    const shifted = await deleteChatFolder(entry.wsId, entry.id, init)
    // The rows the daemon's dense renumber moved that we did not ask about.
    if (shifted.length > 0) store.getState().applyAgentChatFolders(shifted)
  } catch (err) {
    store.getState().applyAgentChatFolders([previous])
    applyChatMoves(store, orphans)
    throw err
  }
}

/** A chat: gone, and every thread and folder under it with it. */
async function sendSubtreeRemoval(
  store: WorkspaceStore,
  entry: RemovalEntry,
  init?: RequestInit,
): Promise<void> {
  const st = store.getState()
  const chatIds = new Set(st.agentChats.chats.map((c) => c.id))
  // `hiddenIds` is already deepest-first with the subject last (planChatRemoval).
  const doomedChats = entry.hiddenIds.filter((id) => chatIds.has(id))
  const doomedFolders = entry.hiddenIds.filter((id) => !chatIds.has(id))
  // Membership sets BESIDE the ordered lists, never instead of them: the ORDER of
  // `doomedChats` is load-bearing (deepest-first, so a parent never goes before
  // the threads hanging off it), while the three tests below only ask whether a
  // row is in the set — and as `includes` each of them rescanned the whole list
  // once per row it was tested against.
  const doomedChatIds = new Set(doomedChats)
  const doomedFolderIds = new Set(doomedFolders)

  const previous = st.agentChats.chats.filter((c) => doomedChatIds.has(c.id))
  const previousFolders = st.agentChats.folders.filter((f) => doomedFolderIds.has(f.id))
  const wasActive = st.agentChats.activeChatId
  // Captured as {id, chatId, name} rather than as buffers: the restore below
  // reopens them by chat, and carrying the union through would make it re-narrow
  // a type it has already established.
  const openBuffers = st.buffers.flatMap((b) =>
    b.type === 'agentChat' && doomedChatIds.has(b.chatId)
      ? [{ id: b.id, chatId: b.chatId, name: b.name }]
      : [],
  )

  for (const id of doomedChats) st.removeAgentChat(id)
  for (const id of doomedFolders) st.removeAgentChatFolder(id)
  for (const buffer of openBuffers) {
    // Raw closeBuffer is NOT enough — it filters the buffer out of the array but
    // leaves the owning pane's activeBufferId pointing at it, so deleting the
    // chat you were looking at blanks the whole pane. removeBufferFromPane
    // activates an adjacent tab first; this is the exact closeTab pattern the
    // WS-stream hook documents.
    for (const pane of Object.values(st.panes)) {
      if (pane.bufferIds.includes(buffer.id)) {
        st.paneActions.removeBufferFromPane(pane.id, buffer.id)
      }
    }
    st.bufferActions.closeBuffer(buffer.id)
  }

  try {
    for (const id of doomedChats) {
      // react-doctor-disable-next-line async-await-in-loop -- deliberate: a subtree is deleted deepest-first, one aggregate write at a time; firing them together lets a parent's delete land before a child's and orphan the child.
      await deleteChat(entry.wsId, id, init)
    }
  } catch (err) {
    const snap = store.getState()
    for (const chat of previous) snap.upsertAgentChat(chat)
    if (previousFolders.length > 0) snap.applyAgentChatFolders(previousFolders)
    // The optimistic close took the chats' tabs with it — put them back too, or a
    // chat snaps into the list with its open tab silently gone.
    for (const buffer of openBuffers) {
      snap.bufferActions.openContent({
        type: 'agentChat',
        chatId: buffer.chatId,
        wsId: entry.wsId,
        name: buffer.name,
      })
    }
    if (wasActive !== null && previous.some((c) => c.id === wasActive)) {
      snap.setActiveAgentChatId(wasActive)
    }
    throw err
  }
}
