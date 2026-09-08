import { EMPTY_CHATS, EMPTY_FOLDERS, type Chat, type Folder } from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarChat } from '@/components/layout/workspace-tree-utils'
import {
  foldOwningChats,
  homeOwningChatId,
  resolveOwnership,
  walkTreeIntoRows,
} from '@/components/sidebar/lib/rows-from-repo'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * The one placement fact a repo's own header row needs to interleave
 * correctly among this project's home chats/folders — its row id (the
 * repo's default-workspace id, the same id `rowsFromRepo`'s own header push
 * uses), the project-home folder it is filed under ('' for root), and its
 * raw `Repository.Order`. Nothing else: `rowsFromHome` never builds a row
 * for a repo (that stays `rowsFromRepo`'s job, unchanged) — it only computes
 * WHERE that row belongs, by giving it a seat in the same sibling sort its
 * neighbors already go through.
 */
export interface HomeRepoPlacement {
  id: string
  folderId: string
  order: number
}

const EMPTY_REPO_PLACEMENTS: readonly HomeRepoPlacement[] = []

/** A repo's corrected position within this project's home tree — see
 *  {@link HomeRepoPlacement} and {@link rowsFromHome}'s own doc. */
export interface HomeRepoPosition {
  parentId: string | null
  order: number
}

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
 * `repos` is every repo the caller wants interleaved into this SAME sibling
 * sort — a repo's own row is never built here (it stays `rowsFromRepo`'s
 * header push, whole and unchanged), but its raw `order`/`folderId` are fed
 * into the identical `buildSidebarTree` call this function already makes for
 * chats/folders, as a position-only stand-in `walkTreeIntoRows` recognises
 * and skips drawing a row for (see its own doc). This is the fix for a real,
 * caught-live bug: a repo's row kept its RAW backend `order` while every
 * chat/folder row here had its OWN `order` field silently REPLACED by a
 * freshly-computed positional index, local to this function's own walk and
 * blind to repos entirely — two independently-dense integer sequences
 * sharing one visual level, which `SidebarTree`'s `roots.sort(byOrder)`
 * could only ever compare by coincidence, not by the position a drag
 * actually promised. Feeding repos into the SAME walk that produces that
 * positional index is what makes the two agree — the second return value is
 * the corrected `{parentId, order}` for each repo id, which the caller
 * applies to the row `rowsFromRepo` already built before assembling the
 * final list (`space-scroller.tsx`'s `SpacePanel`).
 */
export function rowsFromHome(
  homeWorkspaceId: string,
  chats: Chat[] = EMPTY_CHATS,
  folders: Folder[] = EMPTY_FOLDERS,
  repos: readonly HomeRepoPlacement[] = EMPTY_REPO_PLACEMENTS,
): { rows: SidebarRow[]; repoPositions: ReadonlyMap<string, HomeRepoPosition> } {
  const rows: SidebarRow[] = []
  const chatTitleById = new Map(chats.map((c) => [c.id, c.title]))
  const ownership = resolveOwnership([], chats)
  const { ownerOfChat } = ownership

  const homeRowId = homeOwningChatId(chats, homeWorkspaceId)

  // Position-only stand-ins: same id as the repo's own row (its default
  // workspace), no title (never rendered), placed exactly the way that row's
  // own `parentId`/`order` already are — see rows-from-repo.ts's home-row
  // push, which this has to agree with.
  const repoStandins: SidebarChat[] = repos.map((r) => ({
    id: r.id,
    parentId: r.folderId || undefined,
    order: r.order,
    title: '',
  }))
  const repoPositions = new Map<string, HomeRepoPosition>(
    repos.map((r) => [r.id, { parentId: null, order: 0 }]),
  )

  const roots = buildSidebarTree(
    [],
    folders,
    [...chats.filter((c) => c.id !== homeRowId), ...repoStandins],
  )
  const folded = foldOwningChats(roots, ownership)

  // `false`: project home has no repo, so there is no worktree for any of
  // its folders to fork — see `foldersCanFork`'s own doc on `walkTreeIntoRows`.
  walkTreeIntoRows(rows, folded, null, ownerOfChat, chatTitleById, false, repoPositions)

  return { rows, repoPositions }
}
