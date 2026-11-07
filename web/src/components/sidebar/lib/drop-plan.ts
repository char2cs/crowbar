import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { Repo } from '@/core/entities/repo-types'
import {
  levelWorkspaceOfBranchRow,
  resolveRowRepo,
} from '@/components/sidebar/lib/sidebar-drop-policy'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { homeOwnerRowId } from '@/features/workspace/tree/legacy-tree-index'
import { getHomeTree, resolveHomeRowScope } from '@/core/entities/home-rows'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
} from '@/features/workspace/lib/home-workspace-resolver'
import { planTreeRowDrop, type RowPlacementCall } from '@/components/sidebar/lib/drop-plan-tree'
import {
  insertIndex,
  projectOfWorkspace,
  renderedProjectRows,
  renderedSiblings,
  visibleRepos,
} from '@/components/sidebar/lib/rendered-rows'

/** A chat subject whose target lives in a different workspace — no endpoint
 *  can re-home a chat to another workspace's own aggregate (see `planChatDrop`). */
export const UNSUPPORTED = 'unsupported' as const

// A chat drop. Chat placement lives on the workspace-scoped AgentChat aggregate
// (setChatPlacement). A chat lands beside or into a chat or folder, or past a branch row on its
// own level — never into a branch. Indices come from the drawn rows, which hold all three kinds.
function planChatDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  if (target.kind !== 'chat' && target.kind !== 'folder' && target.kind !== 'branch') return []
  if (target.kind === 'branch' && mode === 'into') return []
  // A folder/branch carries no chat workspace of its own — the dragged
  // chat's OWN workspace is the only ground such a drop can mean, and every
  // subject is required to share it (checked right below).
  const destWorkspaceId = target.kind === 'chat' ? target.workspaceId : subjects[0].workspaceId
  if (!destWorkspaceId) return []
  // `setChatPlacement` is scoped to one workspace's own chat tree — there is
  // no field on it that re-homes a chat to a different workspace's aggregate.
  if (subjects.some((s) => s.workspaceId !== destWorkspaceId)) return UNSUPPORTED

  const repos = visibleRepos()
  if (target.kind === 'branch' && levelWorkspaceOfBranchRow(repos, target) !== destWorkspaceId) {
    return UNSUPPORTED
  }
  const projectId =
    resolveHomeRowScope(target.id)?.projectId ??
    resolveRowRepo(repos, target.id)?.projectId ??
    projectOfWorkspace(repos, destWorkspaceId)
  if (!projectId) return []

  const rows = renderedProjectRows(projectId)
  // 'after' an EXPANDED chat/folder is the slot before its first child — where
  // the line is drawn (`resolvesToFirstChild`). Never for a branch target: its
  // children are another workspace's level, which a chat cannot join.
  const firstChild =
    target.kind !== 'branch' &&
    resolvesToFirstChild(
      {
        kind: target.kind,
        id: target.id,
        expanded: !treeUiStore.getState().collapsed.has(target.id),
        hasChildren: rows.some((r) => r.parentId === target.id),
      },
      mode,
    )
  // A Recents-sourced drop never reaches here (`performSidebarDrop` branches
  // it off to `performRecentsDrop` first), so a root-level target genuinely
  // means the tree root.
  const containerId = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
  if (folderOutsideTree(repos, containerId, destWorkspaceId)) return UNSUPPORTED
  const lifted = new Set(subjects.map((s) => s.id))
  const rest = renderedSiblings(rows, containerId, lifted)
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, target.id, mode)

  return subjects.map((subject, i) => ({
    kind: 'chat' as const,
    workspaceId: destWorkspaceId,
    chatId: subject.id,
    parentId: containerId,
    order: at + i,
  }))
}

// A folder belongs to one tree, its repo's or its project home's, and a chat can only be filed
// in its own workspace's tree: anywhere else the daemon keeps a parent the sidebar never draws.
function folderOutsideTree(repos: readonly Repo[], folderId: string, workspaceId: string): boolean {
  const home = resolveHomeRowScope(folderId)
  if (home?.kind === 'folder') return home.homeWorkspaceId !== workspaceId
  const folderRepo = repos.find((r) => r.folders?.some((f) => f.id === folderId))
  return !!folderRepo && resolveRowRepo(repos, workspaceId)?.repoId !== folderRepo.id
}

// A project-home folder drop: planTreeRowDrop's container/index math scoped to the home. Home
// never forks, so only folders reach here; the home's owning chat is the level, not a row in it.
function planHomeFolderDrop(
  projectId: string,
  homeWorkspaceId: string,
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const tree = getHomeTree(projectId)
  // A repo header is a root sibling to reorder past, never a container.
  if (target.kind === 'branch' && mode === 'into') return []
  const homeRowId = homeOwnerRowId(
    homeWorkspaceId,
    tree.chats,
    getHomeOwningChatId(projectId) ?? undefined,
  )
  // A home chat or folder; the owner is the home level itself, never a row in it.
  const isHomeNode = (id: string) =>
    tree.folders.some((f) => f.id === id) ||
    (id !== homeRowId && tree.chats.some((c) => c.id === id))
  const targetId = target.id
  // Siblings come from the rows the panel draws: at the home root a repo's header is one.
  const rows = renderedProjectRows(projectId)
  const hasChildren = rows.some((r) => r.parentId === target.id)
  const expanded = !treeUiStore.getState().collapsed.has(target.id)
  // Only a home chat/folder has a first-child slot: an expanded repo header holds its own branches.
  const firstChild =
    mode !== 'into' &&
    isHomeNode(targetId) &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requested = mode === 'into' || firstChild ? targetId : target.parentId || ''
  const containerId = requested !== '' && !isHomeNode(requested) ? '' : requested

  const lifted = new Set(subjects.map((s) => s.id))
  const rest = renderedSiblings(rows, containerId, lifted)
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, targetId, mode)

  const calls: RowPlacementCall[] = []
  subjects.forEach((row, i) => {
    // Only a home folder is planned (the policy should already have refused anything else).
    if (row.kind !== 'folder' || !tree.folders.some((f) => f.id === row.id)) return
    // A reorder within its current container sends no `parentId`: re-sending one can trip
    // checkFolderContextMove for a drop that changed no parent.
    const sameContainer = requested === (row.parentId ?? '')
    calls.push({
      kind: 'homeFolder',
      projectId,
      folderId: row.id,
      ...(sameContainer ? {} : { parentId: containerId }),
      order: at + i,
    })
  })
  return calls
}

// Which project's home a target belongs to: a home chat/folder via resolveHomeRowScope, or a repo
// header via its repoIcon (which resolveHomeRowScope doesn't know about).
function targetProjectHomeScope(target: SidebarRow): { projectId: string } | null {
  const homeScope = resolveHomeRowScope(target.id)
  if (homeScope) return { projectId: homeScope.projectId }
  if (target.repoIcon) return { projectId: target.repoIcon.projectId }
  return null
}

// A repo header drop. Its placement lives on the Repository aggregate, so planTreeRowDrop can't
// address it. It may only reorder among, or file into a folder of, its project's home; the policy
// refuses everything else and this is the backstop.
function planRepoHomeDrop(
  subject: SidebarRow,
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const repoIcon = subject.repoIcon
  if (!repoIcon) return []
  if (mode === 'into' && target.kind !== 'folder') return []
  const scope = targetProjectHomeScope(target)
  if (!scope) return []
  if (!getHomeWorkspaceId(scope.projectId)) return []
  const containerId = mode === 'into' ? target.id : (target.parentId ?? '')
  // Only home root or a home folder, never a chat's thread space (backstop for the policy).
  if (containerId !== '' && resolveHomeRowScope(containerId)?.kind !== 'folder') return []
  // Lifted by its ROW id (the header row is id'd by the checkout's owning
  // chat when it has one), the same id space `target.id` is looked up in.
  const rest = renderedSiblings(
    renderedProjectRows(scope.projectId),
    containerId,
    new Set([subject.id]),
  )
  const at = mode === 'into' ? rest.length : insertIndex(rest, target.id, mode)
  return [
    {
      kind: 'repoHome',
      projectId: scope.projectId,
      repoId: repoIcon.repoId,
      folderId: containerId,
      order: at,
    },
  ]
}

export function planRowDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  if (subjects.length === 0) return []
  if (subjects[0].kind === 'chat') return planChatDrop(subjects, target, mode)
  // Home never forks, so a home subject is always a folder; repo and home folder ids share the
  // daemon's id space and never collide, so checking the first subject is safe.
  const homeScope = resolveHomeRowScope(subjects[0].id)
  if (homeScope) {
    return planHomeFolderDrop(
      homeScope.projectId,
      homeScope.homeWorkspaceId,
      subjects,
      target,
      mode,
    )
  }
  // A repo header, only ever dragged alone, gets its own plan (Repository aggregate).
  if (subjects.length === 1 && subjects[0].repoIcon) {
    return planRepoHomeDrop(subjects[0], target, mode)
  }
  return planTreeRowDrop(subjects, target, mode)
}
