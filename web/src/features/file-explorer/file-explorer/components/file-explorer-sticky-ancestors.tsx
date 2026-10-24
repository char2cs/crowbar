import type React from 'react'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import type { VisibleFileTreeRow } from '@/features/file-explorer/lib/visible-file-tree-rows'
import type { FileEntry } from '@/features/file-system/types/app'
import {
  SIDEBAR_FILE_ROW_BASE_INDENT,
  SIDEBAR_FILE_ROW_ICON_SIZE,
  SidebarFileRow,
} from '@/components/sidebar/sidebar-file-row'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { DirectoryChevron } from './file-explorer-tree-item'

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
  rowHeight,
  getGitStatusDecoration,
}: {
  ancestors: VisibleFileTreeRow[]
  style: React.CSSProperties
  containerInset: number
  indentSize: number
  rowHeight: number
  getGitStatusDecoration: (file: FileEntry) => FileTreeGitStatusDecoration | null
}) {
  return (
    <div className="file-tree-sticky-ancestors" style={style}>
      <div className="file-tree-sticky-ancestor-stack">
        {ancestors.map((ancestor) => (
          <SidebarFileRow
            key={ancestor.file.path}
            data-file-path={ancestor.file.path}
            data-is-dir={ancestor.file.isDir}
            data-path={ancestor.file.path}
            data-depth={ancestor.depth}
            title={ancestor.file.path}
            depth={ancestor.depth}
            indentSize={indentSize}
            baseIndent={SIDEBAR_FILE_ROW_BASE_INDENT + containerInset}
            height={rowHeight}
            leading={<DirectoryChevron isDir isExpanded />}
            icon={
              <FileExplorerIcon
                fileName={ancestor.file.name}
                isDir={ancestor.file.isDir ?? false}
                isExpanded={ancestor.isExpanded}
                isSymlink={ancestor.file.isSymlink}
                className="shrink-0 text-muted-foreground"
                size={SIDEBAR_FILE_ROW_ICON_SIZE}
              />
            }
            label={ancestor.displayName ?? ancestor.file.name}
            labelClassName={getGitStatusDecoration(ancestor.file)?.colorClassName}
          />
        ))}
      </div>
    </div>
  )
}
