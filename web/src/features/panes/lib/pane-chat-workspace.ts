import {
  resolveChatOwnerWorkspaceId,
  resolveWorkspaceIdForChat,
} from '@/features/workspace/stores/workspace-store-registry'

/**
 * THE workspace a chat belongs to — never "whichever workspace happens to be
 * routed right now".
 *
 * Panes are window-level (Task 26) but a chat's own state is not: it lives in
 * exactly one workspace store, and every chat-scoped URL is built from that
 * workspace's id. Until this existed, the pane render path had no way to ask
 * the question at all — `PaneContainer` handed `AgentChatPane` the AMBIENT
 * `WorkspaceStoreContext`'s id, i.e. whichever `WorkspaceView` happened to be
 * on screen — so a pane holding another workspace's chat resolved it against
 * the wrong store. That is the gap `openChatIntoPane`'s active-workspace
 * refusal stood in for, and the reason a drag from any row outside the routed
 * workspace silently did nothing.
 *
 * Three sources, strongest first:
 *
 *   1. **The chat record's own `workspaceId`**, off whichever registered store
 *      carries it ({@link resolveChatOwnerWorkspaceId}). Authoritative: it is
 *      the daemon's answer, delivered with the chat.
 *   2. **`hint`** — what the caller's own row already claims (a `SidebarRow`'s
 *      `workspaceId`, built from the sidebar's live tree). The answer while no
 *      workspace store has been seeded with this chat yet, which is routine
 *      for a workspace the user has never opened.
 *   3. **The registry KEY the chat was found under**
 *      ({@link resolveWorkspaceIdForChat}). Deliberately last, and NOT
 *      interchangeable with 1: `listChats` is repo-scoped, so every workspace
 *      store in a repo is seeded with that whole repo's chats and the first
 *      key iterated matches any of them. It is still worth asking — a store
 *      that holds the chat can serve it — but only once the two better
 *      answers have declined.
 *
 * Null means nothing in the app can name a workspace for this chat. Callers
 * skip rather than guess: opening a chat under the wrong workspace is what
 * produced the permanently blank, reload-surviving pane this replaces.
 */
export function resolveChatWorkspaceId(chatId: string, hint?: string | null): string | null {
  return resolveChatOwnerWorkspaceId(chatId) || hint || resolveWorkspaceIdForChat(chatId) || null
}

/**
 * Whether any registered workspace store knows `chatId` as a chat at all.
 *
 * The `hint` clause above is a claim, not a check — it will happily hand back
 * a workspace for an id that names no chat. A caller translating a row id into
 * a chat id (a `branch` row carries the id of the chat that owns its
 * workspace, and falls back to its own workspace id when that chat cannot be
 * resolved — see `rows-from-repo.ts`) needs the check, not the claim.
 */
export function isKnownChatId(chatId: string): boolean {
  return resolveWorkspaceIdForChat(chatId) !== null
}
