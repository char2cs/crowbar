import { useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { SIDEBAR_TREE_ICON_SIZE, SidebarTreeRow } from '@/components/ui/sidebar-tree'
import {
  buildGitFolderTree,
  sortFilesByPath,
  sortFoldersByName,
} from '@/features/git/utils/build-git-folder-tree'
import type { GitFolderNode } from '@/features/git/utils/build-git-folder-tree'
import type { GitDiff, GitFile } from '@/features/git/types/git-types'
import { GitFileItem } from '@/features/git/components/status/git-status-file-item'
import { useSettingsStore } from '@/features/settings/store'
import '@/features/file-explorer/styles/file-explorer-tree.css'

interface ChangedFilesTreeProps {
  files: GitDiff[]
  repoPath?: string
  onFileOpen: (filePath: string) => void
}

/** Pitch of a single row — `SidebarTreeRow` renders a fixed `h-6` button. */
const ROW_HEIGHT = 24

/**
 * Internal shape stored in GitFolderNode.files — extends GitFile with the
 * extra fields we need from GitDiff.
 */
interface ChangedFileEntry extends GitFile {
  additions?: number
  deletions?: number
  uncommitted?: boolean
  /** Original file_path from GitDiff so we call onFileOpen with the right key */
  filePath: string
}

/** One rendered line of the tree, after the nested folder tree is flattened. */
type ChangedFilesRow =
  | { kind: 'folder'; key: string; depth: number; node: GitFolderNode; collapsed: boolean }
  | { kind: 'file'; key: string; depth: number; entry: ChangedFileEntry }

/**
 * Adapt a GitDiff array into the GitFile-compatible shape that
 * buildGitFolderTree expects, while carrying the extra fields we need.
 */
function adaptDiffToEntries(diffs: GitDiff[]): ChangedFileEntry[] {
  return diffs.map((diff) => ({
    // GitFile required fields
    path: diff.file_path,
    status: diff.is_new
      ? 'added'
      : diff.is_deleted
        ? 'deleted'
        : diff.is_renamed
          ? 'renamed'
          : 'modified',
    staged: false,
    // Extra fields for display
    additions: diff.additions,
    deletions: diff.deletions,
    uncommitted: diff.uncommitted,
    filePath: diff.file_path,
  }))
}

/**
 * Flatten the nested folder tree into the exact row order the recursive render
 * used to produce — at every level, each folder followed by its own subtree,
 * then that level's files — so virtualising changes nothing a user can see.
 * A collapsed folder contributes its own row and none of its descendants.
 */
function flattenChangedFilesTree(
  root: GitFolderNode,
  collapsedFolders: ReadonlySet<string>,
): ChangedFilesRow[] {
  const rows: ChangedFilesRow[] = []

  const walk = (node: GitFolderNode, depth: number) => {
    for (const folder of sortFoldersByName(node.folders.values())) {
      const collapsed = collapsedFolders.has(folder.fullPath)
      rows.push({
        kind: 'folder',
        key: `folder:${folder.fullPath}`,
        depth,
        node: folder,
        collapsed,
      })
      if (!collapsed) walk(folder, depth + 1)
    }
    for (const file of sortFilesByPath(node.files)) {
      // Cast to access the extra fields we added in adaptDiffToEntries
      const entry = file as ChangedFileEntry
      rows.push({ kind: 'file', key: `file:${entry.filePath}`, depth, entry })
    }
  }

  walk(root, 0)
  return rows
}

export function ChangedFilesTree({ files, repoPath, onFileOpen }: ChangedFilesTreeProps) {
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
            return (
              <SidebarTreeRow
                key={row.key}
                depth={row.depth}
                onClick={() => toggleFolder(row.node.fullPath)}
                className="min-w-0 leading-[1.35]"
              >
                <FileExplorerIcon
                  fileName={row.node.name}
                  isDir
                  isExpanded={!row.collapsed}
                  className="relative z-1 shrink-0 text-muted-foreground"
                  size={SIDEBAR_TREE_ICON_SIZE}
                />
                <span className="relative z-1 truncate leading-[1.35]">{row.node.name}</span>
              </SidebarTreeRow>
            )
          }

          const entry = row.entry
          const diffStats =
            entry.additions !== undefined || entry.deletions !== undefined
              ? { additions: entry.additions ?? 0, deletions: entry.deletions ?? 0 }
              : undefined

          return (
            <GitFileItem
              key={row.key}
              file={entry}
              diffStats={diffStats}
              onClick={() => onFileOpen(entry.filePath)}
              showDirectory={false}
              showFileIcon
              indentLevel={row.depth}
              repoPath={repoPath}
              uncommitted={entry.uncommitted}
              compactGitStatusBadges={compactGitStatusBadges}
            />
          )
        })}
        <div style={{ height: paddingBottom }} />
      </div>
    </div>
  )
}
