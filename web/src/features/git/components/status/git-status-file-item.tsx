import type { MouseEvent } from 'react'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { Badge } from '@/components/ui/badge'
import { SIDEBAR_TREE_ICON_SIZE, SidebarTreeRow } from '@/components/ui/sidebar-tree'
import { cn } from '@/utils/cn'
import type { GitFile } from '../../types/git-types'

interface GitFileItemProps {
  file: GitFile
  diffStats?: {
    additions: number
    deletions: number
  }
  onClick?: () => void
  onContextMenu?: (e: MouseEvent) => void
  showDirectory?: boolean
  showFileIcon?: boolean
  indentLevel?: number
  className?: string
  repoPath?: string
  uncommitted?: boolean
  /**
   * Required, not defaulted: this used to be read from `useSettingsStore` here,
   * which cost one store subscription per row in a tree that can hold hundreds.
   * The owning list subscribes once and passes it down. Keeping it required
   * makes a missed call site a type error rather than a silent visual change.
   */
  compactGitStatusBadges: boolean
}

export const GitFileItem = ({
  file,
  diffStats,
  onClick,
  onContextMenu,
  showDirectory = true,
  showFileIcon = false,
  indentLevel = 0,
  className,
  repoPath,
  uncommitted,
  compactGitStatusBadges,
}: GitFileItemProps) => {
  const pathParts = file.path.split('/')
  const fileName = pathParts.pop() || file.path
  const directory = pathParts.join('/')
  const hasDiffStats = !!diffStats && (diffStats.additions > 0 || diffStats.deletions > 0)

  return (
    <SidebarTreeRow
      depth={indentLevel}
      className={cn('group min-w-0 leading-[1.35]', className)}
      onClick={onClick}
      onContextMenu={onContextMenu}
      draggable={!!repoPath}
    >
      {showFileIcon && (
        <FileExplorerIcon
          fileName={fileName}
          isDir={false}
          className="relative z-1 shrink-0 text-muted-foreground"
          size={SIDEBAR_TREE_ICON_SIZE}
        />
      )}
      <div
        className="relative z-1 flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden"
        title={file.path}
      >
        <span
          className={cn(
            'min-w-0 truncate leading-[1.35]',
            showDirectory ? 'shrink-0 basis-auto max-w-[45%]' : 'flex-1',
            'text-foreground',
          )}
        >
          {fileName}
        </span>
        {showDirectory && directory && (
          <span className="ui-text-sm min-w-0 flex-1 truncate leading-[1.35] text-muted-foreground/80">
            {directory}
          </span>
        )}
      </div>
      <div className="relative z-1 ml-auto flex shrink-0 items-center gap-1.5">
        {uncommitted && (
          <Badge variant="warning" size="sm">
            uncommitted
          </Badge>
        )}
        {hasDiffStats && (
          <div
            className={cn(
              'flex items-center leading-[1.35]',
              compactGitStatusBadges ? 'ui-text-sm gap-0.5' : 'ui-text-sm gap-1',
            )}
          >
            {diffStats.additions > 0 && (
              <span className="text-git-added">+{diffStats.additions}</span>
            )}
            {diffStats.deletions > 0 && (
              <span className="text-git-deleted">-{diffStats.deletions}</span>
            )}
          </div>
        )}
      </div>
    </SidebarTreeRow>
  )
}
