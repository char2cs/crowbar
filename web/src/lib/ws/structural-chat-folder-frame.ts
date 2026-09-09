import { NON_STRUCTURAL_CHAT_KINDS } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'

/**
 * Whether a frame off a repo's `.../chats/ws` feed names a row that MOVED —
 * a folder placement, a Node-backed workspace/branch placement, or a chat
 * whose kind isn't one of the turn/session housekeeping ones — as opposed to
 * a reconnect sentinel or a runner/liveness frame neither cache cares about.
 *
 * Shared by `app-sync-provider.tsx`'s own tree reseed AND
 * `crowbar_workspaces`'s `shouldReseed` (entity-stream.ts): both caches are
 * derived from the SAME repo-wide chat/folder tree, and both need refreshing
 * off the SAME signal — a `placement_set`/`folder_updated` frame carries no
 * `chatId` at all (see PushAgentChatFolder), so `workspaceDTOFromWorktreeFrame`'s
 * own `worktree_state`-only mapper drops it silently, and only this check
 * (not a per-row merge) can tell the cache it is stale.
 */
export function isStructuralChatFolderFrame(raw: unknown): boolean {
  if (!raw || typeof raw !== 'object') return false
  const ev = raw as { folderId?: string; chatId?: string; kind?: string; runnerId?: string }
  if (ev.runnerId) return false
  if (ev.folderId) return true
  if (!ev.chatId || !ev.kind) return false
  return !NON_STRUCTURAL_CHAT_KINDS.has(ev.kind)
}
