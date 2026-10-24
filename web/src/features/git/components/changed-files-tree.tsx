import { useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { SIDEBAR_TREE_ICON_SIZE, SidebarTreeRow } from '@/components/ui/sidebar-tree'
import {
  aggregateFolders,
  buildGitFolderTree,
  collectNodeFiles,
  compactFolder,
  sortFilesByPath,
  sortFoldersByName,
} from '@/features/git/utils/build-git-folder-tree'
import type { GitFolderNode } from '@/features/git/utils/build-git-folder-tree'
import type { GitDiff, GitFile } from '@/features/git/types/git-types'
import { GitFileItem } from '@/features/git/components/status/git-status-file-item'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { isFileViewed } from '@/features/git/lib/review-viewed'
import { useSettingsStore } from '@/features/settings/store'
import '@/features/file-explorer/styles/file-explorer-tree.css'

interface ChangedFilesTreeProps {
  files: GitDiff[]
  repoPath?: string
  onFileOpen: (filePath: string) => void
  /** Path -> signature of every file marked viewed. */
  viewed: Readonly<Record<string, string>>
  /** Mark or unmark a batch of files in one write. */
  onSetViewed: (files: GitDiff[], value: boolean) => void
  /** Unresolved anchored threads per file path. */
  openThreadCounts: ReadonlyMap<string, number>
}

/** Pitch of a single row — `SidebarTreeRow` renders a fixed `h-6` button. */
const ROW_HEIGHT = 24

/** A tree entry: the git file shape the folder builder wants, plus its diff. */
interface ChangedFileEntry extends GitFile {
  diff: GitDiff
}

/** One rendered line of the tree, after the nested folder tree is flattened. */
type ChangedFilesRow =
  | {
      kind: 'folder'
      key: string
      depth: number
      node: GitFolderNode
      label: string
      collapsed: boolean
    }
  | { kind: 'file'; key: string; depth: number; entry: ChangedFileEntry }

function adaptDiffToEntries(diffs: GitDiff[]): ChangedFileEntry[] {
  return diffs.map((diff) => ({
    path: diff.file_path,
    status: diff.is_new
      ? 'added'
      : diff.is_deleted
        ? 'deleted'
        : diff.is_renamed
          ? 'renamed'
          : 'modified',
    staged: false,
    diff,
  }))
}

/**
 * Flatten the nested folder tree into display order — each folder (single-child
 * chains folded into one row) followed by its own subtree, then that level's
 * files. A collapsed folder contributes its own row and none of its descendants.
 */
function flattenChangedFilesTree(
  root: GitFolderNode,
  collapsedFolders: ReadonlySet<string>,
): ChangedFilesRow[] {
  const rows: ChangedFilesRow[] = []

  const walk = (node: GitFolderNode, depth: number) => {
    for (const child of sortFoldersByName(node.folders.values())) {
      const { label, node: folder } = compactFolder(child)
      const collapsed = collapsedFolders.has(folder.fullPath)
      rows.push({
        kind: 'folder',
        key: `folder:${folder.fullPath}`,
        depth,
        node: folder,
        label,
        collapsed,
      })
      if (!collapsed) walk(folder, depth + 1)
    }
    for (const file of sortFilesByPath(node.files)) {
      const entry = file as ChangedFileEntry
      rows.push({ kind: 'file', key: `file:${entry.path}`, depth, entry })
    }
  }

  walk(root, 0)
  return rows
}

export function ChangedFilesTree({
  files,
  repoPath,
  onFileOpen,
  viewed,
  onSetViewed,
  openThreadCounts,
}: ChangedFilesTreeProps) {
  const [collapsedFolders, setCollapsedFolders] = useState<Set<string>>(() => new Set())
  const scrollRef = useRef<HTMLDivElement>(null)

  // Subscribed ONCE here rather than per row: `GitFileItem` used to read this
  // itself, which meant one store subscription per changed file.
  const compactGitStatusBadges = useSettingsStore((state) => state.settings.compactGitStatusBadges)

  // Adapting the diff + rebuilding the nested folder tree is O(files) and only
  // depends on `files`. Memoize it so toggling a folder (or any unrelated
  // re-render) doesn't re-adapt every diff and rebuild the whole tree.
  const rootNode = useMemo(() => {
    const entries = adaptDiffToEntries(files)
    // buildGitFolderTree accepts GitFile[] — ChangedFileEntry extends GitFile so this is safe
    return buildGitFolderTree(entries as GitFile[])
  }, [files])

  // One post-order pass per files/viewed change; rows only look counts up.
  const aggregates = useMemo(
    () =>
      aggregateFolders(rootNode, (file) => isFileViewed(viewed, (file as ChangedFileEntry).diff)),
    [rootNode, viewed],
  )

  const rows = useMemo(
    () => flattenChangedFilesTree(rootNode, collapsedFolders),
    [rootNode, collapsedFolders],
  )

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    estimateSize: () => ROW_HEIGHT,
    getScrollElement: () => scrollRef.current,
    overscan: 8,
    // Mirror useFileExplorerVisibleRows: while a sidebar/pane drag is in progress
    // (data-pane-resizing on <html>) suppress ResizeObserver callbacks — otherwise
    // the drag drives 120fps re-renders — and flush the last deferred measurement
    // on the pane-resize-end event.
    observeElementRect: (instance, cb) => {
      const element = instance.scrollElement
      if (!element) return
      let pending: { width: number; height: number } | null = null
      const ro = new ResizeObserver(([entry]) => {
        const { width, height } = entry.contentRect
        const rect = { width: Math.round(width), height: Math.round(height) }
        if (document.documentElement.hasAttribute('data-pane-resizing')) {
          pending = rect
          return
        }
        cb(rect)
        pending = null
      })
      ro.observe(element)
      const r = element.getBoundingClientRect()
      cb({ width: Math.round(r.width), height: Math.round(r.height) })
      const flush = () => {
        if (pending) {
          cb(pending)
          pending = null
        }
      }
      window.addEventListener('pane-resize-end', flush)
      return () => {
        ro.disconnect()
        window.removeEventListener('pane-resize-end', flush)
      }
    },
  })

  const toggleFolder = (folderPath: string) => {
    setCollapsedFolders((prev) => {
      const next = new Set(prev)
      if (next.has(folderPath)) {
        next.delete(folderPath)
      } else {
        next.add(folderPath)
      }
      return next
    })
  }

  const virtualRows = rowVirtualizer.getVirtualItems()
  const paddingTop = virtualRows.length > 0 ? virtualRows[0].start : 0
  const paddingBottom =
    virtualRows.length > 0
      ? rowVirtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end
      : 0

  return (
    <div ref={scrollRef} className="h-full min-h-0 select-none overflow-y-auto">
      <div className="flex flex-col">
        <div style={{ height: paddingTop }} />
        {virtualRows.map((virtualRow) => {
          const row = rows[virtualRow.index]
          if (!row) return null

          if (row.kind === 'folder') {
            const agg = aggregates.get(row.node.fullPath) ?? { total: 0, viewed: 0 }
            const all = agg.total > 0 && agg.viewed === agg.total
            return (
              <SidebarTreeRow
                key={row.key}
                depth={row.depth}
                onClick={() => toggleFolder(row.node.fullPath)}
                className="min-w-0 leading-[1.35]"
                leading={
                  <Checkbox
                    ariaLabel={`Mark ${row.label} as viewed`}
                    checked={all}
                    indeterminate={agg.viewed > 0 && !all}
                    onChange={(next: boolean) =>
                      onSetViewed(
                        collectNodeFiles(row.node).map((f) => (f as ChangedFileEntry).diff),
                        next,
                      )
                    }
                  />
                }
              >
                <FileExplorerIcon
                  fileName={row.node.name}
                  isDir
                  isExpanded={!row.collapsed}
                  className="relative z-1 shrink-0 text-muted-foreground"
                  size={SIDEBAR_TREE_ICON_SIZE}
                />
                <span className="relative z-1 min-w-0 flex-1 truncate leading-[1.35]">
                  {row.label}
                </span>
                <span className="ui-text-sm relative z-1 shrink-0 text-muted-foreground">
                  {agg.viewed}/{agg.total}
                </span>
              </SidebarTreeRow>
            )
          }

          const { diff } = row.entry
          const isViewed = isFileViewed(viewed, diff)
          const threads = openThreadCounts.get(diff.file_path) ?? 0
          const diffStats =
            diff.additions !== undefined || diff.deletions !== undefined
              ? { additions: diff.additions ?? 0, deletions: diff.deletions ?? 0 }
              : undefined

          return (
            <GitFileItem
              key={row.key}
              file={row.entry}
              diffStats={diffStats}
              onClick={() => onFileOpen(diff.file_path)}
              showDirectory={false}
              showFileIcon
              indentLevel={row.depth}
              repoPath={repoPath}
              uncommitted={diff.uncommitted}
              compactGitStatusBadges={compactGitStatusBadges}
              dimmed={isViewed}
              leading={
                <Checkbox
                  ariaLabel={`Mark ${row.entry.path.split('/').pop()} as viewed`}
                  checked={isViewed}
                  onChange={(next: boolean) => onSetViewed([diff], next)}
                />
              }
              trailing={
                threads > 0 ? (
                  <Badge
                    variant="info"
                    size="sm"
                    aria-label={`${threads} open thread${threads === 1 ? '' : 's'}`}
                  >
                    {threads}
                  </Badge>
                ) : null
              }
            />
          )
        })}
        <div style={{ height: paddingBottom }} />
      </div>
    </div>
  )
}
