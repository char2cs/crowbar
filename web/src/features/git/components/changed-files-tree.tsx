import type React from 'react'
import { Fragment, useState } from 'react'
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
import '@/features/file-explorer/styles/file-explorer-tree.css'

interface ChangedFilesTreeProps {
  files: GitDiff[]
  repoPath?: string
  onFileOpen: (filePath: string) => void
}

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

export function ChangedFilesTree({ files, repoPath, onFileOpen }: ChangedFilesTreeProps) {
  const [collapsedFolders, setCollapsedFolders] = useState<Set<string>>(new Set())

  const entries = adaptDiffToEntries(files)
  // buildGitFolderTree accepts GitFile[] — ChangedFileEntry extends GitFile so this is safe
  const rootNode = buildGitFolderTree(entries as GitFile[])

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

  const renderNode = (node: GitFolderNode, depth: number): React.ReactNode => {
    const folderRows = sortFoldersByName(node.folders.values()).map((folderNode) => {
      const isCollapsed = collapsedFolders.has(folderNode.fullPath)

      return (
        <Fragment key={folderNode.fullPath}>
          <SidebarTreeRow
            depth={depth}
            onClick={() => toggleFolder(folderNode.fullPath)}
            className="min-w-0 leading-[1.35]"
          >
            <FileExplorerIcon
              fileName={folderNode.name}
              isDir
              isExpanded={!isCollapsed}
              className="relative z-1 shrink-0 text-muted-foreground"
              size={SIDEBAR_TREE_ICON_SIZE}
            />
            <span className="relative z-1 truncate leading-[1.35]">{folderNode.name}</span>
          </SidebarTreeRow>
          {!isCollapsed ? renderNode(folderNode, depth + 1) : null}
        </Fragment>
      )
    })

    // Cast to access the extra fields we added in adaptDiffToEntries
    const fileRows = sortFilesByPath(node.files).map((file) => {
      const entry = file as ChangedFileEntry
      const diffStats =
        entry.additions !== undefined || entry.deletions !== undefined
          ? { additions: entry.additions ?? 0, deletions: entry.deletions ?? 0 }
          : undefined

      return (
        <GitFileItem
          key={entry.filePath}
          file={entry}
          diffStats={diffStats}
          onClick={() => onFileOpen(entry.filePath)}
          showDirectory={false}
          showFileIcon
          indentLevel={depth}
          repoPath={repoPath}
          uncommitted={entry.uncommitted}
        />
      )
    })

    return (
      <>
        {folderRows}
        {fileRows}
      </>
    )
  }

  return <div className="flex flex-col select-none">{renderNode(rootNode, 0)}</div>
}
