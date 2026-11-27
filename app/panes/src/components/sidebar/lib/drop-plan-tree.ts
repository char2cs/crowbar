import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveChatRepo, resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { resolveHomeOwnerId } from '@/components/sidebar/lib/workspace-owners'
import {
  insertIndex,
  renderedProjectRows,
  renderedSiblings,
  visibleRepos,
} from '@/components/sidebar/lib/rendered-rows'
import { UserFacingError } from '@/lib/errors/user-facing-error'

export type RowPlacementCall =
  | { kind: 'reparent'; projectId: string; repoId: string; wsId: string; parentId: string }
  | {
      kind: 'workspace'
      projectId: string
      repoId: string
      wsId: string
      folderId?: string
      order: number
    }
  | {
      kind: 'folder'
      projectId: string
      repoId: string
      folderId: string
      parentId?: string
      order: number
    }
  | {
      kind: 'chat'
      workspaceId: string
      chatId: string
      parentId: string
      order: number
      moveTo?: { workspaceId: string; label: string }
    }
  | {
      kind: 'homeFolder'
      projectId: string
      folderId: string
      parentId?: string
      order: number
    }
  | { kind: 'repoHome'; projectId: string; repoId: string; folderId: string; order: number }

function containerKindOf(repo: Repo, id: string): 'workspace' | 'folder' | undefined {
  if (repo.workspaces.some((w) => w.id === id)) return 'workspace'
  return repo.folders?.some((f) => f.id === id) ? 'folder' : undefined
}

function workspaceAnchor(repo: Repo, containerId: string): string {
  const workspaceIds = new Set(repo.workspaces.map((w) => w.id))
  const folderById = new Map((repo.folders ?? []).map((f) => [f.id, f]))
  const visited = new Set<string>()
  let cursor = containerId
  while (cursor !== '' && !visited.has(cursor)) {
    if (workspaceIds.has(cursor)) return cursor
    const owned = workspaceIdOfBranchRow([repo], cursor)
    if (owned) return owned
    visited.add(cursor)
    cursor = folderById.get(cursor)?.parentId ?? ''
  }
  return ''
}

export function planTreeRowDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const { collapsed } = treeUiStore.getState()
  const repos = visibleRepos()
  const scope =
    resolveRowRepo(repos, target.id) ??
    (target.kind === 'chat' ? resolveChatRepo(repos, target.id) : null)
  const repo = scope && repos.find((r) => r.id === scope.repoId)
  if (!repo || !scope?.projectId) return []
  const { repoId, projectId } = scope

  const wsSpace = (id: string) => workspaceIdOfBranchRow(repos, id) ?? id
  const targetId = wsSpace(target.id)

  const rows = renderedProjectRows(projectId)
  const hasChildren = rows.some((r) => r.parentId === target.id)
  const expanded = !collapsed.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requestedRow = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
  const requested = requestedRow === '' ? '' : wsSpace(requestedRow)
  const found = requested === '' ? undefined : containerKindOf(repo, requested)
  const containerId = found === undefined ? '' : requested
  const containerKind = found ?? 'root'
  const homeRowId = repo.defaultWorkspaceId
    ? resolveHomeOwnerId(repo.defaultWorkspaceId, repo.defaultOwningChatId, repo.chats ?? [])
    : ''
  const containerRowId = containerId === '' ? homeRowId : requestedRow

  const lifted = new Set(subjects.map((s) => s.id))
  const rest = renderedSiblings(rows, containerRowId, lifted).map(wsSpace)
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, targetId, mode)

  const calls: RowPlacementCall[] = []
  subjects.forEach((row, i) => {
    const order = at + i
    const subject = { kind: row.kind, id: wsSpace(row.id) }
    if (subject.id === repo.defaultWorkspaceId) return

    if (subject.kind === 'folder') {
      if (!repo.folders?.some((f) => f.id === subject.id)) return
      const sameContainer = requestedRow === (row.parentId ?? '')
      calls.push({
        kind: 'folder',
        projectId,
        repoId,
        folderId: subject.id,
        ...(sameContainer ? {} : { parentId: containerId }),
        order,
      })
      return
    }
    if (subject.kind !== 'branch') return

    const ws = repo.workspaces.find((w) => w.id === subject.id)
    if (!ws) return
    const currentFork = ws.parentId ?? ''
    const forked = currentFork !== '' && repo.workspaces.some((w) => w.id === currentFork)
    const visibleCurrentFork = forked ? currentFork : ''
    const visibleNextFork = containerKind === 'root' ? '' : workspaceAnchor(repo, containerId)
    const nextFork =
      visibleNextFork === visibleCurrentFork
        ? currentFork
        : visibleNextFork || repo.defaultWorkspaceId || ''
    const folderId =
      containerKind === 'folder' ? containerId : containerKind === 'root' ? '' : undefined

    if (nextFork !== '' && nextFork !== currentFork) {
      if (repo.workspaces.find((w) => w.id === nextFork)?.provisioning === 'placeholder') {
        throw new UserFacingError("That branch hasn't been checked out yet — try again once it has")
      }
      const reparentFolderId = containerKind === 'folder' ? containerId : undefined
      calls.push({ kind: 'reparent', projectId, repoId, wsId: subject.id, parentId: nextFork })
      calls.push({
        kind: 'workspace',
        projectId,
        repoId,
        wsId: subject.id,
        ...(reparentFolderId !== undefined && { folderId: reparentFolderId }),
        order,
      })
      return
    }

    const directFolderId =
      containerKind === 'workspace'
        ? (owningChatIdOfWorkspace(repos, containerId) ?? containerId)
        : folderId
    calls.push({
      kind: 'workspace',
      projectId,
      repoId,
      wsId: subject.id,
      ...(directFolderId !== undefined && { folderId: directFolderId }),
      order,
    })
  })

  return calls
}
