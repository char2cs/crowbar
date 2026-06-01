import { useMemo } from 'react'
import { FilePlus, FileX, FileText, PencilSimple } from '@phosphor-icons/react'
import {
  TreeProvider,
  TreeView,
  TreeNode,
  TreeNodeTrigger,
  TreeExpander,
  TreeIcon,
  TreeLabel,
  TreeNodeContent,
} from '@/components/kibo-ui/tree'
import type { GitDiff } from '@/features/git/types/git-types'
import { cn } from '@/utils/cn'

/** Stable DOM id for a file's diff section, so the tree can scroll to it. */
export function diffFileAnchorId(filePath: string): string {
  return `diff-file-${filePath.replace(/[^a-zA-Z0-9]/g, '-')}`
}

interface FileNode {
  name: string
  path: string
  isFile: boolean
  diff?: GitDiff
  children: FileNode[]
}

function buildTree(files: GitDiff[]): { roots: FileNode[]; folderIds: string[] } {
  const root: FileNode = { name: '', path: '', isFile: false, children: [] }
  const folderIds: string[] = []

  for (const diff of files) {
    const parts = diff.file_path.split('/')
    let cursor = root
    let acc = ''
    parts.forEach((part, i) => {
      acc = acc ? `${acc}/${part}` : part
      const isFile = i === parts.length - 1
      let child = cursor.children.find(c => c.name === part && c.isFile === isFile)
      if (!child) {
        child = { name: part, path: acc, isFile, diff: isFile ? diff : undefined, children: [] }
        cursor.children.push(child)
        if (!isFile) folderIds.push(acc)
      }
      cursor = child
    })
  }

  // Sort: folders first, then files, alphabetically — at every level.
  const sortRec = (node: FileNode) => {
    node.children.sort((a, b) =>
      a.isFile === b.isFile ? a.name.localeCompare(b.name) : a.isFile ? 1 : -1,
    )
    node.children.forEach(sortRec)
  }
  sortRec(root)

  return { roots: root.children, folderIds }
}

function fileStatusIcon(diff: GitDiff) {
  if (diff.is_new) return <FilePlus className="text-git-added" />
  if (diff.is_deleted) return <FileX className="text-git-deleted" />
  if (diff.is_renamed) return <PencilSimple className="text-git-modified" />
  return <FileText className="text-git-modified" />
}

interface DiffFileTreeProps {
  files: GitDiff[]
  onSelectFile: (filePath: string) => void
}

export function DiffFileTree({ files, onSelectFile }: DiffFileTreeProps) {
  const { roots, folderIds } = useMemo(() => buildTree(files), [files])

  const renderNode = (node: FileNode, level: number, isLast: boolean): React.ReactNode => {
    const hasChildren = node.children.length > 0
    return (
      <TreeNode key={node.path} nodeId={node.path} level={level} isLast={isLast}>
        <TreeNodeTrigger
          onClick={() => { if (node.isFile) onSelectFile(node.path) }}
        >
          <TreeExpander hasChildren={hasChildren} />
          <TreeIcon hasChildren={hasChildren} icon={node.isFile && node.diff ? fileStatusIcon(node.diff) : undefined} />
          <TreeLabel>{node.name}</TreeLabel>
          {node.isFile && node.diff && (
            <span className="ml-auto flex shrink-0 items-center gap-1.5 pl-2 font-mono text-[10px]">
              {(node.diff.additions ?? 0) > 0 && (
                <span className="text-git-added">+{node.diff.additions}</span>
              )}
              {(node.diff.deletions ?? 0) > 0 && (
                <span className="text-git-deleted">−{node.diff.deletions}</span>
              )}
            </span>
          )}
        </TreeNodeTrigger>
        {hasChildren && (
          <TreeNodeContent hasChildren={hasChildren}>
            {node.children.map((child, i) =>
              renderNode(child, level + 1, i === node.children.length - 1),
            )}
          </TreeNodeContent>
        )}
      </TreeNode>
    )
  }

  return (
    <TreeProvider
      defaultExpandedIds={folderIds}
      showLines
      indent={16}
      animateExpand={false}
      className={cn('h-full overflow-y-auto py-2')}
    >
      <TreeView>
        {roots.map((node, i) => renderNode(node, 0, i === roots.length - 1))}
      </TreeView>
    </TreeProvider>
  )
}
