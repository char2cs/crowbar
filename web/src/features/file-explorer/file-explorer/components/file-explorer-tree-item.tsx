import type React from 'react'
import { memo, useEffect, useRef } from 'react'
import {
  FILE_TREE_DENSITY_CONFIG,
  type FileTreeDensity,
} from '@/features/file-explorer/lib/file-tree-density'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import type { FileEntry } from '@/features/file-system/types/app'
import { Input } from '@/components/ui/input'
import { TreeRow } from '@/components/ui/tree-row'
import { cn } from '@/utils/cn'
import { FileExplorerIcon } from './file-explorer-icon'

export const FILE_TREE_BASE_INDENT = 10

export interface FileTreeGuideTarget {
  path: string
  name: string
  isDir: boolean
  isActive: boolean
}

function areGuideTargetsEqual(
  previous: Array<FileTreeGuideTarget | null>,
  next: Array<FileTreeGuideTarget | null>,
): boolean {
  if (previous.length !== next.length) return false

  return previous.every((previousTarget, index) => {
    const nextTarget = next[index]
    if (previousTarget === nextTarget) return true
    if (!previousTarget || !nextTarget) return false

    return (
      previousTarget.path === nextTarget.path &&
      previousTarget.name === nextTarget.name &&
      previousTarget.isDir === nextTarget.isDir &&
      previousTarget.isActive === nextTarget.isActive
    )
  })
}

interface FileExplorerTreeItemProps {
  file: FileEntry
  depth: number
  displayName?: string
  guideTargets: Array<FileTreeGuideTarget | null>
  previousDepth: number
  nextDepth: number
  indentSize: number
  density: FileTreeDensity
  isExpanded: boolean
  isActive: boolean
  dragOverPath: string | null
  isDragging: boolean
  editingValue: string
  onEditingValueChange: (value: string) => void
  onKeyDown: (e: React.KeyboardEvent, file: FileEntry) => void
  onBlur: (file: FileEntry) => void
  getGitStatusDecoration: (file: FileEntry) => FileTreeGitStatusDecoration | null
  searchQuery?: string
  isSearchMatch?: boolean
  rowId?: string
  fileFeedback?: Map<string, 'copied-path' | 'copied-rel' | 'created' | 'err'>
}

function renderHighlightedLabel(label: string, query: string | undefined) {
  const trimmedQuery = query?.trim()
  if (!trimmedQuery) return label

  const labelLower = label.toLowerCase()
  const queryLower = trimmedQuery.toLowerCase()
  const matchIndex = labelLower.indexOf(queryLower)

  if (matchIndex === -1) return label

  return (
    <>
      {label.slice(0, matchIndex)}
      <mark className="file-tree-search-highlight">
        {label.slice(matchIndex, matchIndex + trimmedQuery.length)}
      </mark>
      {label.slice(matchIndex + trimmedQuery.length)}
    </>
  )
}

// react-doctor-disable-next-line no-many-boolean-props -- these flags (isExpanded, isActive, isDragging, isSearchMatch) are orthogonal render states of a single virtualized tree row, driven per-row by the virtualizer. They are not mutually exclusive, so they can't collapse to one enum; splitting into named variants would mean a 2^4 combinatorial explosion of row components for no benefit.
function FileExplorerTreeItemComponent({
  file,
  depth,
  displayName,
  guideTargets,
  previousDepth,
  nextDepth,
  indentSize,
  density,
  isExpanded,
  isActive,
  dragOverPath,
  isDragging,
  editingValue,
  onEditingValueChange,
  onKeyDown,
  onBlur,
  getGitStatusDecoration,
  searchQuery,
  isSearchMatch = false,
  rowId,
  fileFeedback,
}: FileExplorerTreeItemProps) {
  const isCut = useFileClipboardStore(
    (s) =>
      s.clipboard?.operation === 'cut' && s.clipboard.entries.some((e) => e.path === file.path),
  )
  const paddingLeft = FILE_TREE_BASE_INDENT + depth * indentSize
  const densityConfig = FILE_TREE_DENSITY_CONFIG[density]
  const gitStatusDecoration = getGitStatusDecoration(file)
  const guideLevels = Array.from({ length: depth }, (_, level) => level)
  /**
   * `oracleAnchors` is false in the inline-editing branch: that branch renders a
   * different row (an `<Input>` where the button is), and tagging it would put a
   * second `file-row-item` in the document under a shape the native surface does
   * not model. The extractor takes the first match for its root, so the two
   * would be indistinguishable.
   *
   * **That branch is not anchor-free, though, and cannot be made so.** The
   * `<Input>` it renders carries the `input` surface's own `input-control` /
   * `input` anchors. They harm nothing today — an editing row is a *sibling* of
   * every `file-row-item` and `extractSnapshot` walks only downwards from its
   * root — but they are why the editing cell can never simply be tagged into
   * `file-tree-row`. The full reasoning, and why ANCHORS.md v1.8's declared
   * scope is not available to this surface, is in
   * `native/mapping/file-tree-row.md`.
   */
  const renderTreeGuides = (oracleAnchors: boolean) => (
    <div className="file-tree-guides">
      {guideLevels.map((level) => {
        const target = guideTargets[level]
        const startsHere = previousDepth <= level
        const endsHere = nextDepth <= level
        return (
          <span
            key={level}
            className="file-tree-guide"
            data-oracle-id={oracleAnchors ? `file-row-guide-${level}` : undefined}
            data-file-path={target?.path}
            data-is-dir={target?.isDir}
            data-path={target?.path}
            data-active={target?.isActive ? 'true' : undefined}
            title={target?.name}
            style={
              {
                left: `calc(${FILE_TREE_BASE_INDENT + level * indentSize}px + var(--file-tree-guide-icon-offset, 7px))`,
                top: startsHere ? '4px' : '0',
                bottom: endsHere ? '4px' : '0',
              } as React.CSSProperties
            }
          />
        )
      })}
    </div>
  )

  const inputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (!file.isEditing && !file.isRenaming) return
    const el = inputRef.current
    if (!el) return
    // Defer focus by one frame so any cleanup effects from unmounting components
    // (e.g. @base-ui/react/menu restoring focus on close) settle first. Without
    // this, the menu cleanup fires after el.focus() and returns focus to body,
    // which immediately triggers handleBlur → cancelInlineEditing.
    const timer = setTimeout(() => {
      if (el.isConnected) {
        el.focus()
        el.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'nearest' })
      }
    }, 16)
    return () => clearTimeout(timer)
  }, [file.isEditing, file.isRenaming])

  if (file.isEditing || file.isRenaming) {
    return (
      <div className="file-tree-item w-full" data-depth={depth}>
        {renderTreeGuides(false)}
        <div
          className={cn(
            'file-tree-row flex w-full items-center rounded-md',
            densityConfig.rowClassName,
          )}
          style={{
            paddingLeft: `${paddingLeft}px`,
          }}
        >
          <FileExplorerIcon
            fileName={file.isDir ? 'folder' : 'file'}
            isDir={file.isDir ?? false}
            isExpanded={false}
            className="relative z-[1] shrink-0 text-muted-foreground"
          />
          <Input
            ref={inputRef}
            type="text"
            autoCapitalize="none"
            autoComplete="off"
            autoCorrect="off"
            spellCheck="false"
            value={editingValue}
            onFocus={() => {
              if (file.isRenaming) {
                onEditingValueChange(file.name)
              }
            }}
            onChange={(e) => onEditingValueChange(e.target.value)}
            onKeyDown={(e) => onKeyDown(e, file)}
            onBlur={() => onBlur(file)}
            className="ui-font relative z-1 flex-1 border-foreground border-b px-0 focus:border-muted-foreground"
            placeholder={file.isDir ? 'folder name' : 'file name'}
          />
        </div>
      </div>
    )
  }

  return (
    /*
      The `file-tree-row` oracle surface — `native/oracle/ANCHORS.md`. Every
      anchor below is an inert marker: no styling, no behaviour, and no
      conditional rendering hangs off one. They ship in production by design,
      exactly as the git status row's do.

      This row is the surface the §8.3 **state** axis is measured on, and the
      reason is right here on the next line: `data-active` is set from `isActive`
      on every render, so `.file-tree-item[data-active='true']::before` really
      paints `var(--accent)`. `SidebarTreeRow` takes an `active` prop that
      neither live consumer passes, which makes the same attribute dead on the
      git status row and every `selected` cell of its matrix vacuous.
    */
    <div
      className="file-tree-item w-full"
      data-active={isActive ? 'true' : undefined}
      data-depth={depth}
      data-oracle-id="file-row-item"
    >
      {renderTreeGuides(true)}
      <TreeRow
        id={rowId}
        data-oracle-id="file-row-button"
        role="treeitem"
        aria-level={depth + 1}
        aria-selected={isActive}
        aria-expanded={file.isDir ? isExpanded : undefined}
        data-file-path={file.path}
        data-is-dir={file.isDir}
        data-path={file.path}
        data-depth={depth}
        title={
          file.isSymlink && file.symlinkTarget ? `Symlink to: ${file.symlinkTarget}` : undefined
        }
        className={cn(
          densityConfig.rowClassName,
          dragOverPath === file.path &&
            '!border-2 !border-dashed !border-secondary !bg-secondary !bg-opacity-20',
          isDragging && 'cursor-move',
          file.ignored && 'opacity-50',
          isCut && 'italic opacity-40',
          isSearchMatch && 'file-tree-search-match',
        )}
        active={isActive}
        baseIndent={FILE_TREE_BASE_INDENT}
        depth={depth}
        indentSize={indentSize}
      >
        <FileExplorerIcon
          fileName={file.name}
          isDir={file.isDir ?? false}
          isExpanded={isExpanded}
          isSymlink={file.isSymlink}
          className="relative z-1 shrink-0 text-muted-foreground"
          data-oracle-id="file-row-icon"
        />
        <span className="relative z-1 flex min-w-0 items-baseline gap-1.5">
          {/*
            `data-oracle-content-sized` (ANCHORS.md v1.5): this span is a flex
            item with `flex: 0 1 auto` and no basis, so while the row has room
            its used width *is* its max-content width — the fraction this engine
            keeps and GPUI `ceil()`s. It stays correct in the `overflow` cell
            too: there the width is what the icon and the whole-pixel paddings
            leave, so the reference is an integer and `ceil` of it is itself.

            `data-oracle-line-sized` (v1.6): a blockified flex item holding one
            line with no authored height, so its border box is its line box.
            **The line box here is 20px, not the git row's 18.9.** `GitFileItem`
            authors `leading-[1.35]`; this row authors nothing, so it inherits
            `.text-sm`'s `calc(1.25 / 0.875)` — 14px text on a 20px line.

            The button is deliberately *not* line-sized even though it paints
            text: `h-6` authors its box at 24px around that same 20px line.

            The same two declarations are on the GPUI side in `crowbar-ui`'s
            `file_tree_row::{CONTENT_SIZED, LINE_SIZED}`. A declaration on one
            side only is a `FieldPresence` delta that forgives nothing.
          */}
          <span
            className={cn(
              'select-none truncate whitespace-nowrap',
              gitStatusDecoration?.colorClassName,
            )}
            data-oracle-id="file-row-name"
            data-oracle-content-sized="true"
            data-oracle-line-sized="true"
          >
            {renderHighlightedLabel(displayName ?? file.name, searchQuery)}
          </span>
          {gitStatusDecoration && (
            <span
              className={cn(
                'shrink-0 select-none font-mono text-[10px] leading-none opacity-80',
                gitStatusDecoration.colorClassName,
              )}
              aria-label={gitStatusDecoration.label}
            >
              {gitStatusDecoration.statusLetter}
            </span>
          )}
        </span>
        {fileFeedback?.get(file.path) === 'copied-path' && (
          <span className="ml-auto text-xs text-green-500 shrink-0">✓ path copied</span>
        )}
        {fileFeedback?.get(file.path) === 'copied-rel' && (
          <span className="ml-auto text-xs text-green-500 shrink-0">✓ rel copied</span>
        )}
        {fileFeedback?.get(file.path) === 'created' && (
          <span className="ml-auto text-xs text-green-500 shrink-0">✓ created</span>
        )}
        {fileFeedback?.get(file.path) === 'err' && (
          <span className="ml-auto text-xs text-destructive shrink-0">✕ failed</span>
        )}
      </TreeRow>
    </div>
  )
}

export const FileExplorerTreeItem = memo(
  FileExplorerTreeItemComponent,
  (prev, next) =>
    prev.file === next.file &&
    prev.depth === next.depth &&
    prev.displayName === next.displayName &&
    areGuideTargetsEqual(prev.guideTargets, next.guideTargets) &&
    prev.previousDepth === next.previousDepth &&
    prev.nextDepth === next.nextDepth &&
    prev.indentSize === next.indentSize &&
    prev.density === next.density &&
    prev.isExpanded === next.isExpanded &&
    prev.isActive === next.isActive &&
    prev.dragOverPath === next.dragOverPath &&
    prev.isDragging === next.isDragging &&
    prev.editingValue === next.editingValue &&
    prev.onEditingValueChange === next.onEditingValueChange &&
    prev.onKeyDown === next.onKeyDown &&
    prev.onBlur === next.onBlur &&
    prev.getGitStatusDecoration === next.getGitStatusDecoration &&
    prev.searchQuery === next.searchQuery &&
    prev.isSearchMatch === next.isSearchMatch &&
    prev.rowId === next.rowId &&
    prev.fileFeedback?.get(prev.file.path) === next.fileFeedback?.get(next.file.path),
)
