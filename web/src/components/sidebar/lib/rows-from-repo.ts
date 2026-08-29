import { EMPTY_FOLDERS, type Repo } from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * Adapts today's Repo/Workspace/Folder shapes (`lib/store/sidebar.ts`) into
 * the flat `SidebarRow[]` `SidebarTree` renders — the bridge named in Task 4.
 * Deleted in Task 15 once rows arrive pre-shaped over the wire.
 *
 * Reuses `buildSidebarTree`'s fork/folder placement and cycle-guard rules
 * rather than re-deriving them: the hierarchy this produces has to agree with
 * the one the tree it replaces already computes and tests.
 *
 * The repo's default (main-worktree) workspace is not a row in
 * `repo.workspaces` — it becomes this tree's one root, exactly as it is the
 * repo header in the tree being retired. Everything `buildSidebarTree` roots
 * (no fork parent, no compatible folder) nests under it; a repo with no
 * default workspace yet simply has no root row for its own workspaces to
 * hang off.
 */
export function rowsFromRepo(repo: Repo): SidebarRow[] {
  const rows: SidebarRow[] = []
  const homeId = repo.defaultWorkspaceId ?? null

  if (homeId) {
    rows.push({
      id: homeId,
      kind: 'branch',
      parentId: null,
      order: repo.order ?? 0,
      label: repo.name,
      ownsWorktree: true,
      workspaceId: homeId,
      working: repo.defaultWorking ?? false,
      hasView: false,
      branchName: repo.defaultBranch,
    })
  }

  const roots = buildSidebarTree(
    repo.workspaces.filter((w) => w.status !== 'deleted'),
    repo.folders ?? EMPTY_FOLDERS,
  )

  const walk = (nodes: SidebarTreeNode[], parentId: string | null) => {
    nodes.forEach((node, order) => {
      if (node.kind === 'folder') {
        rows.push({
          id: node.id,
          kind: 'folder',
          parentId,
          order,
          label: node.folder.name,
          // A folder groups workspaces, not chats — its own "+" always forks a
          // branch, matching the tree being retired.
          ownsWorktree: true,
          workspaceId: null,
          working: false,
          hasView: false,
        })
      } else {
        rows.push({
          id: node.id,
          kind: 'branch',
          parentId,
          order,
          label: node.workspace.branch,
          ownsWorktree: true,
          workspaceId: node.id,
          working: node.workspace.working ?? false,
          hasView: false,
          branchName: node.workspace.branch,
        })
      }
      walk(node.children, node.id)
    })
  }
  walk(roots, homeId)

  return rows
}
