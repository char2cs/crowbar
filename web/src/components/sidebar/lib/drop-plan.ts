import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { projectOfRow, workspaceOfContainer } from '@/components/sidebar/lib/sidebar-drop-policy'
import { readSelect, selectProjects } from '@/core/entities/select'
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

// A chat drop. Chat placement lives on the workspace-scoped AgentChat aggregate (setChatPlacement):
// it lands beside or into any chat, folder or branch row, and the container it lands in names the
// workspace it runs in; another than its own makes the placement a move. Indices come from the
// drawn rows, which hold all three kinds.
function planChatDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  if (target.kind !== 'chat' && target.kind !== 'folder' && target.kind !== 'branch') return []
  const repos = visibleRepos()
  const projectId =
    projectOfRow(repos, target.id) ??
    (target.workspaceId ? projectOfWorkspace(repos, target.workspaceId) : null)
  if (!projectId) return []

  const rows = renderedProjectRows(projectId)
  // 'after' an EXPANDED row is the slot before its first child — where the line is drawn.
  const firstChild = resolvesToFirstChild(
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
  // A chat the tree has not drawn yet still names its own workspace.
  const destWorkspaceId =
    workspaceOfContainer(repos, projectId, containerId) ??
    (containerId === target.id && target.kind === 'chat' ? target.workspaceId : null)
  if (!destWorkspaceId) return []
  const label = containerLabel(rows, projectId, containerId)
  const lifted = new Set(subjects.map((s) => s.id))
  const rest = renderedSiblings(rows, containerId, lifted)
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, target.id, mode)

  return subjects.flatMap((subject, i): RowPlacementCall[] => {
    if (!subject.workspaceId) return []
    const moves = subject.workspaceId !== destWorkspaceId
    return [
      {
        kind: 'chat',
        workspaceId: subject.workspaceId,
        chatId: subject.id,
        parentId: containerId,
        order: at + i,
        ...(moves && { moveTo: { workspaceId: destWorkspaceId, label } }),
      },
    ]
  })
}

// Where a moved chat lands, in the sidebar's words: its container row's, a branch by its branch.
function containerLabel(rows: readonly SidebarRow[], projectId: string, containerId: string) {
  const row = rows.find((r) => r.id === containerId)
  if (row) return (row.kind === 'branch' && !row.repoIcon && row.branchName) || row.label
  return readSelect(selectProjects).find((p) => p.id === projectId)?.name ?? 'the project home'
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
): RowPlacementCall[] {
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
