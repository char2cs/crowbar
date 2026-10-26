import { ChatsCircle, Folder, FolderOpen, GitBranch, Lock } from '@phosphor-icons/react'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

/** The resting glyph of a sidebar row: its folder, branch state, lock or chat mark. */
export function RowGlyph({
  row,
  large,
  expanded,
  activeGround,
}: {
  row: SidebarRowType
  large: boolean
  expanded: boolean
  /** See `SidebarRowProps.activeGround`'s own doc — forwarded to
   *  `WorkspaceBranchIcon`, the one glyph here with hardcoded ambient colors
   *  (`text-foreground`) instead of inheriting from this row's own treeitem
   *  the way every other glyph below already does. */
  activeGround?: boolean
}) {
  const size = large ? 'size-5' : 'size-4'
  if (row.kind === 'folder') {
    return expanded ? (
      <FolderOpen aria-hidden="true" className={size} weight="duotone" />
    ) : (
      <Folder aria-hidden="true" className={size} weight="duotone" />
    )
  }
  // A branch row with a real `Workspace` record behind it (`row.status` —
  // stamped under the identical condition as `locked`, rows-from-repo.ts)
  // draws the SAME icon the workspace switcher already does: locked, a PR
  // state (open/merged/closed/conflicts), deleted — or, ahead of all of
  // those, the amber warning glyph for a branch Crowbar could not provision
  // (`needsProvisioning`, the identical test
  // `placeholder-toast-watcher.tsx` fires its "Couldn't set up ..." toast
  // from — NOT `isPlaceholder`, which is also true for the repo's own
  // checkout holding its own default branch and would alarm forever). A row whose Workspace half has not landed yet (`walkTreeIntoRows`'s
  // "no Workspace record" push — see that push's own comment) carries no
  // `status` and falls through to the locked/plain-branch guess below,
  // exactly as every row did before this delegated anywhere.
  if (row.kind === 'branch' && row.status) {
    return (
      <WorkspaceBranchIcon
        status={row.status}
        isPlaceholder={row.needsProvisioning}
        reason={row.placeholderReason}
        size={size}
        invertedGround={activeGround}
      />
    )
  }
  // A locked/protected branch (the repo/project home, or any other locked
  // branch `rows-from-repo.ts`'s `walk()` mints) draws the Lock mark instead
  // of the plain GitBranch every other worktree-owning row gets, for the rare
  // row above that reached here with no `status` to delegate on.
  // `row.locked` is `rows-from-repo.ts`'s own `Workspace.status === 'locked'`
  // read straight onto the row — every workspace-owning row is now id'd from
  // its owning chat, locked or not, so the id/workspaceId mismatch this used
  // to read off is no longer a signal unique to the locked case.
  if (row.kind === 'branch' && row.locked) {
    return <Lock aria-hidden="true" className={size} weight="fill" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
