import { EMPTY_CHATS, EMPTY_FOLDERS, type Chat, type Folder } from '@/lib/store/sidebar'
import { buildSidebarTree } from '@/components/layout/workspace-tree-utils'
import {
  foldOwningChats,
  homeOwningChatId,
  resolveOwnership,
  walkTreeIntoRows,
} from '@/components/sidebar/lib/rows-from-repo'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * A project's home-workspace chats and folders, as FLAT TOP-LEVEL rows — no
 * container/"Home" row of its own. Explicit user correction: a first attempt
 * drew one (mirroring a repo's own home row, which every OTHER worktree's
 * chats nest under), and it was rejected outright — home's chats/folders sit
 * at the exact same level as a repo itself, not one level deeper inside a
 * synthetic parent nothing asked for.
 *
 * The degenerate case of `rows-from-repo.ts`'s own tree, not a
 * reimplementation of it: project home can never be forked (there is no
 * worktree to clone), so it has no `Workspace[]` of its own —
 * `buildSidebarTree`/`resolveOwnership` are called with `workspaces: []`
 * throughout. `walkTreeIntoRows` then draws its chats/folders with the
 * identical row shape a repo's does, just rooted at `null` (the tree's own
 * top level) instead of a repo's own home-row id.
 *
 * `homeWorkspaceId`'s owning `branch` chat (see `homeOwningChatId`'s own
 * doc — the daemon backfills exactly one, the same wire concept a repo's
 * home row draws off) is excluded from the walk rather than drawn as a row:
 * it carries no title of its own and exists only to be the ground every
 * other home chat/folder is filed against, the same way a repo's default
 * workspace is never a member of `repo.workspaces` either.
 *
 * A repo filed into this project's home (root or a home folder) is NOT
 * drawn here — that stays `rowsFromRepo`'s own header push, whole and
 * unchanged — nor does this function correct its position any more. Task 3
 * moved a repo's `folderId`/`order` onto its own `Node` row, computed
 * server-side against these SAME real chat/folder siblings, so the wire
 * value `rowsFromRepo` already reads is the repo's true position — the
 * client-side stand-in this function used to inject into `buildSidebarTree`
 * (and the `repoPositions` it then read back for the caller to overwrite the
 * repo row with) is gone. A repo interleaves correctly by simply carrying
 * its own real `order`/`parentId` into the same flat row list this returns,
 * the same way a chat or folder row already does.
 */
export function rowsFromHome(
  homeWorkspaceId: string,
  chats: Chat[] = EMPTY_CHATS,
  folders: Folder[] = EMPTY_FOLDERS,
): SidebarRow[] {
  const rows: SidebarRow[] = []
  const chatTitleById = new Map(chats.map((c) => [c.id, c.title]))
  const ownership = resolveOwnership([], chats)
  const { ownerOfChat } = ownership

  const homeRowId = homeOwningChatId(chats, homeWorkspaceId)

  const roots = buildSidebarTree(
    [],
    folders,
    chats.filter((c) => c.id !== homeRowId),
  )
  const folded = foldOwningChats(roots, ownership)

  // `false`: project home has no repo, so there is no worktree for any of
  // its folders to fork — see `foldersCanFork`'s own doc on `walkTreeIntoRows`.
  walkTreeIntoRows(rows, folded, null, ownerOfChat, chatTitleById, false)

  return rows
}
