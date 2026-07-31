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
          data-oracle-id="git-row-icon"
        />
      )}
      <div
        className="relative z-1 flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden"
        title={file.path}
      >
        {/*
          `data-oracle-line-sized` on both spans, per `native/oracle/ANCHORS.md`
          v1.6: each is a blockified flex item holding one line, so its border
          box *is* its line box — which WebKit floors to a whole logical pixel
          (14 × 1.35 = 18.9 → 18) where GPUI snaps it to the device grid (→ 19).
          The differ therefore takes `bounds.h` against `font.line_height`. The
          same four ids are declared on the GPUI side in `crowbar-ui`'s
          `LINE_SIZED`; the badge is deliberately not one of them.
        */}
        <span
          className={cn(
            'min-w-0 truncate leading-[1.35]',
            showDirectory ? 'shrink-0 basis-auto max-w-[45%]' : 'flex-1',
            'text-foreground',
          )}
          data-oracle-id="git-row-name"
          data-oracle-line-sized="true"
        >
          {fileName}
        </span>
        {showDirectory && directory && (
          <span
            className="ui-text-sm min-w-0 flex-1 truncate leading-[1.35] text-muted-foreground/80"
            data-oracle-id="git-row-dir"
            data-oracle-line-sized="true"
          >
            {directory}
          </span>
        )}
      </div>
      <div className="relative z-1 ml-auto flex shrink-0 items-center gap-1.5">
        {/*
          `data-oracle-content-sized` on the trailing group, per
          `native/oracle/ANCHORS.md` v1.5: these three boxes take their width
          from what they say, so GPUI's `ceil()` on a text run's max-content
          width applies and the differ compares them against `ceil(reference)`.
          Declared rather than detected — the same three ids are declared on the
          GPUI side in `crowbar-ui`'s `CONTENT_SIZED`. `git-row-name` is
          deliberately not one of them: it is the flexible sibling that absorbs
          the excess.

          The two counts additionally carry `data-oracle-line-sized` (v1.6):
          they are bare runs, so their height is their line box. **The badge
          does not**, and that is measured rather than assumed: `size="sm"`
          gives it `h-5 sm:h-4`, so its border box is authored at 20px or 16px
          around a 13.33px line box. Declaring it would compare 16 against
          13.33 and invent a delta on an anchor where the archived gate run has
          both engines at exactly 16.
        */}
        {uncommitted && (
          <Badge
            variant="warning"
            size="sm"
            data-oracle-id="git-row-badge"
            data-oracle-content-sized="true"
          >
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
              <span
                className="text-git-added"
                data-oracle-id="git-row-added"
                data-oracle-content-sized="true"
                data-oracle-line-sized="true"
              >
                +{diffStats.additions}
              </span>
            )}
            {diffStats.deletions > 0 && (
              <span
                className="text-git-deleted"
                data-oracle-id="git-row-deleted"
                data-oracle-content-sized="true"
                data-oracle-line-sized="true"
              >
                -{diffStats.deletions}
              </span>
            )}
          </div>
        )}
      </div>
    </SidebarTreeRow>
  )
}
