import { useSidebarStore } from '@/lib/store/sidebar'
import type { Chat, Repo } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import type { HomeTree } from '@/lib/store/home-tree'

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
 * Absence from `repos` is NOT a "no". A repo whose chat seed has not landed, or
 * whose section is folded away and has no subscription open at all, carries no
 * `chats` (see `Repo.chats`) — a chat no repo in the tree can name yet answers
 * `false`, so a genuine branch's affordances stay put rather than blinking out
 * while the daemon catches up.
 *
 * A project-home chat is a different, PERMANENT case, not a loading gap: it
 * rides no repo at all (`home-tree.ts`'s own doc), so it structurally can
 * never own a worktree — there is no "not yet" to wait out. Any chat found in
 * `homeTrees` (`useHomeTreeStore`'s per-project map, entirely separate from
 * `repos`) therefore answers `true` unconditionally, the moment it is found,
 * rather than falling through to the `repos`-only loading default.
 */
export function chatIsThreadIn(
  repos: readonly Repo[],
  chatId: string | null,
  homeTrees: Record<string, HomeTree> = {},
): boolean {
  if (!chatId) return false
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat) return !ownsWorktree(repo, chat)
  }
  return chatIsInAnyHomeTree(homeTrees, chatId)
}

/**
 * `chatId` among any project's home chats. Mirrors `resolveHomeRowScope`'s own
 * `Object.keys(trees)` scan (home-tree.ts) rather than importing it directly:
 * that resolver also matches folders, returns a `{ kind, projectId,
 * homeWorkspaceId }` shape this call site has no use for, and fails closed
 * (returns `null`) when `getHomeWorkspaceId` can't resolve — a strictness this
 * boolean has no room for, since a home chat is a thread regardless of whether
 * its home workspace id happens to be resolvable yet.
 */
function chatIsInAnyHomeTree(homeTrees: Record<string, HomeTree>, chatId: string): boolean {
  for (const projectId of Object.keys(homeTrees)) {
    if (homeTrees[projectId].chats.some((c) => c.id === chatId)) return true
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

/** {@link chatIsThreadIn} in the render path, fed by both stores a chat can
 *  live in: `useSidebarStore`'s `repos[]` for a repo chat, and
 *  `useHomeTreeStore`'s `trees` for a project-home chat (home rides no repo,
 *  so it is never in the former). Narrow selectors on each, per this repo's
 *  store convention, so a write to either store that leaves this chat's
 *  membership unchanged still re-renders once the identity of the touched
 *  slice changes — but never on writes to unrelated slices of either store. */
export function useChatIsThread(chatId: string | null): boolean {
  const repos = useSidebarStore((s) => s.repos)
  const homeTrees = useHomeTreeStore((s) => s.trees)
  return chatIsThreadIn(repos, chatId, homeTrees)
}
