import { CaretRight } from '@phosphor-icons/react'
import type React from 'react'
import { memo, useEffect, useRef } from 'react'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import type { FileEntry } from '@/features/file-system/types/app'
import { Input } from '@/components/ui/input'
import {
  SIDEBAR_FILE_ROW_BASE_INDENT,
  SIDEBAR_FILE_ROW_ICON_SIZE,
  SidebarFileRow,
  TreeGuides,
  type TreeGuideTarget,
} from '@/components/sidebar/sidebar-file-row'
import { cn } from '@/utils/cn'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { GitStatusLetter } from './git-status-letter'

function areGuideTargetsEqual(
  previous: Array<TreeGuideTarget | null>,
  next: Array<TreeGuideTarget | null>,
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
  guideTargets: Array<TreeGuideTarget | null>
  previousDepth: number
  nextDepth: number
  indentSize: number
  rowHeight: number
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

// Reserves the same leading slot on every row (file or folder) so icons line
// up in one column — only directories draw a caret into it, rotated 90° open.
export function DirectoryChevron({ isDir, isExpanded }: { isDir: boolean; isExpanded: boolean }) {
  return (
    <span
      aria-hidden="true"
      data-testid="file-tree-chevron"
      className="flex size-3.5 shrink-0 items-center justify-center"
    >
      {isDir && (
        <CaretRight
          weight="bold"
          className={cn(
            'size-2.5 text-muted-foreground transition-transform duration-150',
            isExpanded && 'rotate-90',
          )}
        />
      )}
    </span>
  )
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
  rowHeight,
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
  const gitStatusDecoration = getGitStatusDecoration(file)
  const guides = (
    <TreeGuides
      depth={depth}
      baseIndent={SIDEBAR_FILE_ROW_BASE_INDENT}
      indentSize={indentSize}
      previousDepth={previousDepth}
      nextDepth={nextDepth}
      targets={guideTargets}
    />
  )
  const rowGeometry = { depth, indentSize, height: rowHeight, guides }

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
      <SidebarFileRow
        {...rowGeometry}
        leading={<DirectoryChevron isDir={file.isDir ?? false} isExpanded={false} />}
        icon={
          <FileExplorerIcon
            fileName={file.isDir ? 'folder' : 'file'}
            isDir={file.isDir ?? false}
            isExpanded={false}
            className="shrink-0 text-muted-foreground"
            size={SIDEBAR_FILE_ROW_ICON_SIZE}
          />
        }
        editor={
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
            className="ui-font flex-1 border-foreground border-b px-0 focus:border-muted-foreground"
            placeholder={file.isDir ? 'folder name' : 'file name'}
          />
        }
      />
    )
  }

  const feedback = fileFeedback?.get(file.path)

  return (
    <SidebarFileRow
      {...rowGeometry}
      id={rowId}
      role="treeitem"
      aria-level={depth + 1}
      aria-selected={isActive}
      aria-expanded={file.isDir ? isExpanded : undefined}
      data-file-path={file.path}
      data-is-dir={file.isDir}
      data-path={file.path}
      data-depth={depth}
      title={file.isSymlink && file.symlinkTarget ? `Symlink to: ${file.symlinkTarget}` : undefined}
      selected={isActive}
      dropTarget={dragOverPath === file.path}
      className={cn(
        isDragging && 'cursor-move',
        file.ignored && 'opacity-50',
        isCut && 'italic opacity-40',
        isSearchMatch && 'file-tree-search-match',
      )}
      leading={<DirectoryChevron isDir={file.isDir ?? false} isExpanded={isExpanded} />}
      icon={
        <FileExplorerIcon
          fileName={file.name}
          isDir={file.isDir ?? false}
          isExpanded={isExpanded}
          isSymlink={file.isSymlink}
          className="shrink-0 text-muted-foreground"
          size={SIDEBAR_FILE_ROW_ICON_SIZE}
        />
      }
      label={renderHighlightedLabel(displayName ?? file.name, searchQuery)}
      labelClassName={gitStatusDecoration?.colorClassName}
      badge={gitStatusDecoration && <GitStatusLetter decoration={gitStatusDecoration} />}
      trailing={feedback && <FileFeedback feedback={feedback} />}
    />
  )
}

const FEEDBACK_LABEL = {
  'copied-path': '✓ path copied',
  'copied-rel': '✓ rel copied',
  created: '✓ created',
  err: '✕ failed',
} as const

function FileFeedback({ feedback }: { feedback: keyof typeof FEEDBACK_LABEL }) {
  return (
    <span className={cn('text-xs', feedback === 'err' ? 'text-destructive' : 'text-green-500')}>
      {FEEDBACK_LABEL[feedback]}
    </span>
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
    prev.rowHeight === next.rowHeight &&
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
