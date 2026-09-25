import { CaretRight } from '@phosphor-icons/react'
import type React from 'react'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import type { VisibleFileTreeRow } from '@/features/file-explorer/lib/visible-file-tree-rows'
import type { FileEntry } from '@/features/file-system/types/app'
import { cn } from '@/utils/cn'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { SIDEBAR_TREE_ICON_SIZE } from '@/components/ui/sidebar-tree'
import { FILE_TREE_BASE_INDENT } from './file-explorer-tree-item'

/**
 * The folders pinned above the rows while you scroll inside them. They are
 * always expanded (that's why they're pinned), so the caret is always open.
 * Rows carry `data-file-path`, so the container's delegated handlers serve
 * them like any other row.
 */
export function FileExplorerStickyAncestors({
  ancestors,
  style,
  containerInset,
  indentSize,
  rowClassName,
  getGitStatusDecoration,
}: {
  ancestors: VisibleFileTreeRow[]
  style: React.CSSProperties
  containerInset: number
  indentSize: number
  rowClassName: string
  getGitStatusDecoration: (file: FileEntry) => FileTreeGitStatusDecoration | null
}) {
  return (
    <div className="file-tree-sticky-ancestors" style={style}>
      <div className="file-tree-sticky-ancestor-stack">
        {ancestors.map((ancestor) => (
          <button
            key={ancestor.file.path}
            type="button"
            data-file-path={ancestor.file.path}
            data-is-dir={ancestor.file.isDir}
            data-path={ancestor.file.path}
            data-depth={ancestor.depth}
            title={ancestor.file.path}
            className={cn(
              'file-tree-row ui-font ui-text-sm flex w-full min-w-max cursor-pointer select-none items-center whitespace-nowrap rounded-none border-none bg-transparent text-left text-foreground outline-none transition-colors duration-150 hover:bg-muted focus:outline-none',
              rowClassName,
            )}
            style={{
              paddingLeft: `${FILE_TREE_BASE_INDENT + containerInset + ancestor.depth * indentSize}px`,
            }}
          >
            <span aria-hidden="true" className="flex size-3.5 shrink-0 items-center justify-center">
              <CaretRight weight="bold" className="size-2.5 rotate-90 text-muted-foreground" />
            </span>
            <FileExplorerIcon
              fileName={ancestor.file.name}
              isDir={ancestor.file.isDir ?? false}
              isExpanded={ancestor.isExpanded}
              isSymlink={ancestor.file.isSymlink}
              className="relative z-1 shrink-0 text-muted-foreground"
              size={SIDEBAR_TREE_ICON_SIZE}
            />
            <span
              className={cn(
                'relative z-1 select-none whitespace-nowrap',
                getGitStatusDecoration(ancestor.file)?.colorClassName,
              )}
            >
              {ancestor.displayName ?? ancestor.file.name}
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
