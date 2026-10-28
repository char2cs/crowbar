import { ChatsCircle, Folder, FolderOpen, GitBranch, Lock } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { EditableRepoIcon } from '@/components/layout/repo-icon-mark'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { selectEntityBusy, useOperationsStore } from '@/lib/store/operations'

/**
 * What a row's glyph box holds: the flip-dot spinner while the row works —
 * an agent turn, or an operation naming its workspace — else its icon.
 */
export function SidebarRowGlyph({
  row,
  isProjectHome,
  expanded,
  activeGround,
}: {
  row: SidebarRowType
  isProjectHome: boolean
  expanded: boolean
  activeGround?: boolean
}) {
  const busy = useOperationsStore((s) =>
    row.workspaceId ? selectEntityBusy(s, row.workspaceId) : false,
  )
  if (row.working || busy) return <FlickerSpinner className="size-3.5" />
  if (isProjectHome && row.repoIcon) {
    return (
      <EditableRepoIcon
        repo={row.repoIcon}
        projectId={row.repoIcon.projectId}
        repoId={row.repoIcon.repoId}
        size="lg"
      />
    )
  }
  return (
    <RowGlyph row={row} large={isProjectHome} expanded={expanded} activeGround={activeGround} />
  )
}

export function RowGlyph({
  row,
  large,
  expanded,
  activeGround,
}: {
  row: SidebarRowType
  large: boolean
  expanded: boolean
  /** Forwarded to `WorkspaceBranchIcon`, the one glyph with ambient colours. */
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
  // A branch row with its Workspace landed draws the switcher's status icon;
  // `needsProvisioning`, not `isPlaceholder`, since a repo's own checkout is one too.
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
  // A locked branch whose Workspace has not landed yet still reads as locked.
  if (row.kind === 'branch' && row.locked) {
    return <Lock aria-hidden="true" className={size} weight="fill" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
