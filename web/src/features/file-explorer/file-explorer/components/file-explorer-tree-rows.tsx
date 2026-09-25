import type React from 'react'
import { useMemo, type ComponentProps } from 'react'
import {
  computeStickyScrollLayout,
  findTopVisibleItemIndex,
  getGuideAncestorRows,
  getStickyAncestorRows,
  type VisibleFileTreeRow,
} from '@/features/file-explorer/lib/visible-file-tree-rows'
import { FILE_TREE_DENSITY_CONFIG } from '@/features/file-explorer/lib/file-tree-density'
import { useSettingsStore } from '@/features/settings/store'
import type { useFileExplorerVisibleRows } from '../hooks/use-file-explorer-visible-rows'
import { FileExplorerStickyAncestors } from './file-explorer-sticky-ancestors'
import { FileExplorerTreeItem } from './file-explorer-tree-item'

const FILE_TREE_CONTAINER_INSET = 4
const FILE_TREE_HEADER_HEIGHT = 32

export const getFileTreeRowId = (path: string) =>
  `file-tree-row-${path.replace(/[^a-zA-Z0-9_-]/g, '_')}`

type ItemProps = ComponentProps<typeof FileExplorerTreeItem>

interface FileExplorerTreeRowsProps {
  visibleRows: VisibleFileTreeRow[]
  rowVirtualizer: ReturnType<typeof useFileExplorerVisibleRows>['rowVirtualizer']
  activePath?: string
  highlightedPath?: string
  /** The search bar sits inside the scroll container, above the sticky rows. */
  searchBarOpen: boolean
  searchQuery?: string
  matchedPaths: ReadonlySet<string>
  item: Pick<
    ItemProps,
    | 'dragOverPath'
    | 'isDragging'
    | 'editingValue'
    | 'onEditingValueChange'
    | 'onKeyDown'
    | 'onBlur'
    | 'getGitStatusDecoration'
    | 'fileFeedback'
  >
}

/**
 * The virtualized rows with their sticky ancestors. Deliberately not memoized:
 * the virtualizer instance is stable, so a scroll re-renders the tree that owns
 * it and this must follow.
 */
export function FileExplorerTreeRows({
  visibleRows,
  rowVirtualizer,
  activePath,
  highlightedPath,
  searchBarOpen,
  searchQuery,
  matchedPaths,
  item,
}: FileExplorerTreeRowsProps) {
  const fileTreeDensity = useSettingsStore((s) => s.settings.fileTreeDensity)
  const indentSize = useSettingsStore((s) => s.settings.fileTreeIndentSize)

  // Guide targets for every row, recomputed on structural changes only: per-row
  // getGuideAncestorRows in the render loop is O(N×depth) on every scroll.
  const guideTargetsByIndex = useMemo(() => {
    return visibleRows.map((_, i) =>
      getGuideAncestorRows(visibleRows, i).map((ancestor) =>
        ancestor
          ? {
              path: ancestor.file.path,
              name: ancestor.displayName ?? ancestor.file.name,
              isDir: ancestor.file.isDir ?? false,
              isActive: activePath
                ? activePath === ancestor.file.path ||
                  activePath.startsWith(`${ancestor.file.path}/`) ||
                  activePath.startsWith(`${ancestor.file.path}\\`)
                : false,
            }
          : null,
      ),
    )
  }, [visibleRows, activePath])

  const items = rowVirtualizer.getVirtualItems()
  const paddingBottom = items.length
    ? rowVirtualizer.getTotalSize() - items[items.length - 1].end
    : 0
  const densityConfig = FILE_TREE_DENSITY_CONFIG[fileTreeDensity]
  const stickyMarkerIndex = findTopVisibleItemIndex(items, rowVirtualizer.scrollOffset ?? 0)
  const stickyAncestors =
    stickyMarkerIndex >= 0 ? getStickyAncestorRows(visibleRows, stickyMarkerIndex) : []
  const { paddingTop, visibleItems } = computeStickyScrollLayout(
    items,
    stickyMarkerIndex,
    stickyAncestors.length,
    densityConfig.rowHeight,
    FILE_TREE_CONTAINER_INSET,
  )
  const stickyAncestorsStyle = {
    '--file-tree-container-inset': `${FILE_TREE_CONTAINER_INSET}px`,
    // Only the search bar actually occupies FILE_TREE_HEADER_HEIGHT inside the
    // scrollable container; applying it unconditionally locked the sticky
    // ancestor below the viewport's real top and sheared off the row under it.
    '--file-tree-header-height': `${searchBarOpen ? FILE_TREE_HEADER_HEIGHT : 0}px`,
    '--file-tree-sticky-row-height': `${densityConfig.rowHeight}px`,
    '--file-tree-sticky-stack-height': `${stickyAncestors.length * densityConfig.rowHeight}px`,
  } as React.CSSProperties

  return (
    <>
      {stickyAncestors.length > 0 ? (
        <FileExplorerStickyAncestors
          ancestors={stickyAncestors}
          style={stickyAncestorsStyle}
          containerInset={FILE_TREE_CONTAINER_INSET}
          indentSize={indentSize}
          rowClassName={densityConfig.rowClassName}
          getGitStatusDecoration={item.getGitStatusDecoration}
        />
      ) : null}
      <div style={{ height: paddingTop }} />
      {visibleItems.map((vi) => {
        const row = visibleRows[vi.index]
        return (
          <FileExplorerTreeItem
            key={row.file.path}
            {...item}
            file={row.file}
            depth={row.depth}
            displayName={row.displayName}
            guideTargets={guideTargetsByIndex[vi.index] ?? []}
            previousDepth={visibleRows[vi.index - 1]?.depth ?? 0}
            nextDepth={visibleRows[vi.index + 1]?.depth ?? 0}
            indentSize={indentSize}
            density={fileTreeDensity}
            isExpanded={row.isExpanded}
            isActive={highlightedPath === row.file.path}
            rowId={getFileTreeRowId(row.file.path)}
            searchQuery={searchQuery}
            isSearchMatch={matchedPaths.has(row.file.path)}
          />
        )
      })}
      <div style={{ height: paddingBottom }} />
    </>
  )
}
