import { FolderRow } from './folder-row'
import { WorkspaceTreeItem } from './workspace-tree-item'
import type { SidebarRepoTree, SidebarTreeNode } from './workspace-tree-utils'

/**
 * The row dispatch, in one place.
 *
 * Four callers draw a list of tree nodes — a repo's roots, a workspace's
 * children, a folder's children, and the rows a folded parent is holding — and
 * every one of them has to pick the right row type and thread the same seven
 * props through. Kept here so a new prop reaches all four at once.
 */
export interface SidebarNodeProps {
  node: SidebarTreeNode
  depth: number
  repoId: string
  projectId: string
  /** The row this node hangs off: a workspace id, a folder id, or '' at the root. */
  containerId?: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string, projectId: string, repoId: string) => void
  tree: SidebarRepoTree
}

export function SidebarNode({ node, ...rest }: SidebarNodeProps) {
  return node.kind === 'folder' ? (
    <FolderRow node={node} {...rest} />
  ) : (
    <WorkspaceTreeItem node={node} {...rest} />
  )
}

interface HeldRowsProps extends Omit<SidebarNodeProps, 'node' | 'containerId'> {
  /** The outermost kept ids under the folded row, from {@link useKeptRows}. */
  ids: readonly string[]
}

/**
 * The rows a folded parent is still showing.
 *
 * They render one indent step under that parent whatever depth they really live
 * at, and they carry no treatment of their own — a kept row is the same row, it
 * is simply still on screen. Each brings its own subtree, so the drag keeps
 * reading each row's REAL container rather than the parent that hoisted it.
 */
export function HeldRows({ ids, depth, repoId, tree, ...rest }: HeldRowsProps) {
  if (ids.length === 0) return null
  return (
    <div role="group" data-held-rows="">
      {ids.map((id) => {
        const node = tree.nodeById.get(id)
        if (!node) return null
        const parentId = tree.parentById.get(id) ?? ''
        return (
          <SidebarNode
            key={id}
            node={node}
            depth={depth}
            repoId={repoId}
            containerId={parentId === repoId ? '' : parentId}
            tree={tree}
            {...rest}
          />
        )
      })}
    </div>
  )
}
