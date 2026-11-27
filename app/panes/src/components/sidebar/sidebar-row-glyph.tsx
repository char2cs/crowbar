import { ChatsCircle, Folder, FolderOpen, GitBranch, Lock } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { EditableRepoIcon } from '@/components/layout/repo-icon-mark'
import { RepoAvatar } from '@/components/layout/repo-avatar'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { selectEntityBusy, useOperations } from '@/core/entities/operations'

export function SidebarRowGlyph({
  row,
  isProjectHome,
  readOnlyHeader,
  expanded,
  activeGround,
}: {
  row: SidebarRowType
  isProjectHome: boolean
  readOnlyHeader: boolean
  expanded: boolean
  activeGround?: boolean
}) {
  const busy = useOperations((s) =>
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
  if (readOnlyHeader && row.repoIcon) {
    return (
      <RepoAvatar
        avatar={{
          url: row.repoIcon.avatarURL,
          label: row.repoIcon.avatarLabel,
          color: row.repoIcon.avatarColor,
        }}
        name={row.repoIcon.name}
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
  if (row.kind === 'branch' && row.locked) {
    return <Lock aria-hidden="true" className={size} weight="fill" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
