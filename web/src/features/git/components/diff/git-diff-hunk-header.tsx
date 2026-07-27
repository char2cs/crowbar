import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  Minus,
  Plus,
} from '@phosphor-icons/react'
import { memo, useCallback } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { cn } from '@/utils/cn'
import { stageHunk, unstageHunk } from '../../api/git-status-api'
import type { DiffHunkHeaderProps } from '../../types/git-diff-types'
import { createGitHunk } from '../../utils/git-diff-helpers'

const parseHunkHeader = (content: string) => {
  const match = content.match(/@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)/)
  if (!match) return { context: content }
  return {
    oldStart: match[1],
    oldCount: match[2] || '1',
    newStart: match[3],
    newCount: match[4] || '1',
    context: match[5]?.trim() || '',
  }
}

const DiffHunkHeader = memo(
  ({
    hunk,
    isCollapsed,
    onToggleCollapse,
    isStaged,
    filePath,
    onStageHunk,
    onUnstageHunk,
    isInMultiFileView = false,
  }: DiffHunkHeaderProps) => {
    const rootFolderPath = useFileSystemStore((s) => s.rootFolderPath)

    const handleStageHunk = useCallback(
      async (e: React.MouseEvent) => {
        e.stopPropagation()
        if (!rootFolderPath || !filePath) return

        const gitHunk = createGitHunk(hunk, filePath)

        if (isStaged) {
          const success = await unstageHunk(rootFolderPath, gitHunk)
          if (success) {
            window.dispatchEvent(new CustomEvent('git-status-changed'))
            onUnstageHunk?.(gitHunk)
          }
        } else {
          const success = await stageHunk(rootFolderPath, gitHunk)
          if (success) {
            window.dispatchEvent(new CustomEvent('git-status-changed'))
            onStageHunk?.(gitHunk)
          }
        }
      },
      [rootFolderPath, filePath, hunk, isStaged, onStageHunk, onUnstageHunk],
    )

    let additions = 0
    let deletions = 0
    for (const l of hunk.lines) {
      if (l.line_type === 'added') additions++
      else if (l.line_type === 'removed') deletions++
    }

    const headerInfo = parseHunkHeader(hunk.header.content)

    const canStage = !isInMultiFileView && rootFolderPath && filePath

    // The collapse toggle is a real <button> covering the summary, and the
    // stage control is its SIBLING. Previously the whole row was a
    // `role="button"` with the stage button nested inside it, which takes
    // presentational children — assistive tech lost the stage control entirely.
    return (
      <div
        className={cn(
          'group flex items-center justify-between border-border border-b',
          'bg-background px-3 py-1 ui-text-sm leading-5 hover:bg-muted',
        )}
      >
        <button
          type="button"
          aria-expanded={!isCollapsed}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
          onClick={onToggleCollapse}
        >
          {isCollapsed ? (
            <ChevronRight className="text-muted-foreground" />
          ) : (
            <ChevronDown className="text-muted-foreground" />
          )}

          <span className="ui-font text-muted-foreground">
            @@ -{headerInfo.oldStart},{headerInfo.oldCount} +{headerInfo.newStart},
            {headerInfo.newCount} @@
          </span>

          {headerInfo.context && (
            <span className="truncate text-muted-foreground">{headerInfo.context}</span>
          )}
        </button>

        <div className="flex items-center gap-2">
          <div className="ui-text-sm flex items-center gap-1">
            {additions > 0 && <span className="text-git-added">+{additions}</span>}
            {deletions > 0 && <span className="text-git-deleted">-{deletions}</span>}
          </div>

          {canStage && (
            <button
              type="button"
              onClick={handleStageHunk}
              className={cn(
                'flex items-center gap-1 rounded px-1.5 py-0.5 opacity-0 group-hover:opacity-100',
                isStaged
                  ? 'bg-git-deleted/20 text-git-deleted hover:bg-git-deleted/30'
                  : 'bg-git-added/20 text-git-added hover:bg-git-added/30',
              )}
              title={isStaged ? 'Unstage hunk' : 'Stage hunk'}
              aria-label={isStaged ? 'Unstage hunk' : 'Stage hunk'}
            >
              {isStaged ? <Minus /> : <Plus />}
              <span className="ui-text-xs">{isStaged ? 'Unstage' : 'Stage'}</span>
            </button>
          )}
        </div>
      </div>
    )
  },
)

DiffHunkHeader.displayName = 'DiffHunkHeader'

export default DiffHunkHeader
