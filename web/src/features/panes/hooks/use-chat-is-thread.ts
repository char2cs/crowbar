import { useSidebarStore } from '@/lib/store/sidebar'
import type { Chat, Repo } from '@/lib/store/sidebar'

/**
 * Whether the sidebar tree POSITIVELY names `chatId` as a thread — a chat row
 * that owns no worktree of its own and merely runs on the one its parent owns.
 *
 * Branch chrome (the `GitBranch` mark, "Review this branch") belongs to the row
 * that OWNS a worktree, exactly as `RowGlyph` (sidebar-row.tsx) already gates it
 * in the tree. `Chat.workspaceId` cannot stand in for that test — a thread
 * carries its PARENT's, so every thread names a worktree it does not own — and
 * `Chat.type` never could either (see {@link ChatType}: nothing mints a
 * `'branch'`-typed chat any more). That inherited id is exactly how a thread's
 * own header came to draw a branch icon and offer a branch review
 * (live-reported).
 *
 * Absence is NOT a "no". A repo whose chat seed has not landed, or whose
 * section is folded away and has no subscription open at all, carries no
 * `chats` (see `Repo.chats`) — a chat this tree cannot find answers `false`, so
 * a genuine branch's affordances stay put rather than blinking out while the
 * daemon catches up.
 */
export function chatIsThreadIn(repos: readonly Repo[], chatId: string | null): boolean {
  if (!chatId) return false
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat) return !ownsWorktree(repo, chat)
  }
  return false
}

/** Every authority `rows-from-repo.ts` folds a worktree-owning row from: the
 *  chat's own claim (atomic with the row), the repo home's lifted
 *  `owningChatId` (home is never a member of `repo.workspaces`), and the
 *  `Workspace` record's own join for a row cached before `ownsWorktree`. */
function ownsWorktree(repo: Repo, chat: Chat): boolean {
  if (chat.ownsWorktree) return true
  if (repo.defaultOwningChatId && repo.defaultOwningChatId === chat.id) return true
  return repo.workspaces.some((w) => w.owningChatId === chat.id)
}

/** {@link chatIsThreadIn} in the render path. Returns a plain boolean, so every
 *  sidebar write that leaves the answer unchanged re-renders nothing. */
export function useChatIsThread(chatId: string | null): boolean {
  return useSidebarStore((s) => chatIsThreadIn(s.repos, chatId))
}
