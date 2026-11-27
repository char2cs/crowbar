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
  const firstChild = resolvesToFirstChild(
    {
      kind: target.kind,
      id: target.id,
      expanded: !treeUiStore.getState().collapsed.has(target.id),
      hasChildren: rows.some((r) => r.parentId === target.id),
    },
    mode,
  )
  const containerId = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
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

function containerLabel(rows: readonly SidebarRow[], projectId: string, containerId: string) {
  const row = rows.find((r) => r.id === containerId)
  if (row) return (row.kind === 'branch' && !row.repoIcon && row.branchName) || row.label
  return readSelect(selectProjects).find((p) => p.id === projectId)?.name ?? 'the project home'
}

function planHomeFolderDrop(
  projectId: string,
  homeWorkspaceId: string,
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const tree = getHomeTree(projectId)
  if (target.kind === 'branch' && mode === 'into') return []
  const homeRowId = homeOwnerRowId(
    homeWorkspaceId,
    tree.chats,
    getHomeOwningChatId(projectId) ?? undefined,
  )
  const isHomeNode = (id: string) =>
    tree.folders.some((f) => f.id === id) ||
    (id !== homeRowId && tree.chats.some((c) => c.id === id))
  const targetId = target.id
  const rows = renderedProjectRows(projectId)
  const hasChildren = rows.some((r) => r.parentId === target.id)
  const expanded = !treeUiStore.getState().collapsed.has(target.id)
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
    if (row.kind !== 'folder' || !tree.folders.some((f) => f.id === row.id)) return
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

function targetProjectHomeScope(target: SidebarRow): { projectId: string } | null {
  const homeScope = resolveHomeRowScope(target.id)
  if (homeScope) return { projectId: homeScope.projectId }
  if (target.repoIcon) return { projectId: target.repoIcon.projectId }
  return null
}

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
  if (containerId !== '' && resolveHomeRowScope(containerId)?.kind !== 'folder') return []
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
  if (subjects.length === 1 && subjects[0].repoIcon) {
    return planRepoHomeDrop(subjects[0], target, mode)
  }
  return planTreeRowDrop(subjects, target, mode)
}
