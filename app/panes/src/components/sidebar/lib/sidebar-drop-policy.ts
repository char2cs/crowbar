import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
  resolvesToFirstChild,
  type AllowedModes,
  type DropPolicy,
} from '@/components/tree-dnd/drop-core'
import { readSelect, selectRepos } from '@/core/entities/select'
import type { Repo } from '@/core/entities/repo-types'
import { selectEntityBusy, operationsState } from '@/core/entities/operations'
import { isWorkspaceLockedInSidebar } from '@/core/entities/repo-queries'
import { entityStore } from '@/core/entities/entity-store'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { legacyTreeIndex } from '@/features/workspace/tree/legacy-tree-index'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

export interface RowScope {
  repoId: string
  projectId: string | undefined
}

export type DropTargetRow = SidebarRow & { inRecents?: boolean }

export function resolveRowRepo(repos: readonly Repo[], rowId: string): RowScope | null {
  const id = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  const repo = repos.find(
    (r) =>
      r.defaultWorkspaceId === id ||
      r.workspaces.some((w) => w.id === id) ||
      r.folders?.some((f) => f.id === id),
  )
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

export function resolveChatRepo(repos: readonly Repo[], chatId: string): RowScope | null {
  const repo = repos.find((r) => r.chats?.some((c) => c.id === chatId))
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

function repoOfHeaderRow(repos: readonly Repo[], rowId: string): Repo | null {
  const workspaceId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  return repos.find((r) => r.defaultWorkspaceId === workspaceId) ?? null
}

function nearestBranchAnchor(repo: Repo, rowId: string): string {
  const index = legacyTreeIndex([repo], null)
  const seen = new Set<string>()
  let cursor = workspaceIdOfBranchRow([repo], rowId) ?? rowId
  while (cursor !== '' && !seen.has(cursor)) {
    seen.add(cursor)
    const node = index.node(cursor)
    if (!node || node.kind === 'repo') return ''
    if (node.kind === 'workspace') return cursor
    cursor = node.parentId
  }
  return ''
}

export function projectOfRow(repos: readonly Repo[], rowId: string): string | null {
  return (
    resolveHomeRowScope(rowId)?.projectId ??
    resolveRowRepo(repos, rowId)?.projectId ??
    resolveChatRepo(repos, rowId)?.projectId ??
    null
  )
}

export function workspaceOfContainer(
  repos: readonly Repo[],
  projectId: string | null,
  containerId: string,
): string | null {
  if (containerId === '') return projectId ? getHomeWorkspaceId(projectId) : null
  const home = resolveHomeRowScope(containerId)
  if (home) return home.homeWorkspaceId
  const owned = workspaceIdOfBranchRow(repos, containerId)
  if (owned) return owned
  for (const repo of repos) {
    if (repo.defaultWorkspaceId === containerId) return containerId
    if (repo.workspaces.some((w) => w.id === containerId)) return containerId
    if (repo.folders?.some((f) => f.id === containerId)) {
      const anchor = nearestBranchAnchor(repo, containerId)
      if (anchor === '') return repo.defaultWorkspaceId ?? null
      return workspaceIdOfBranchRow(repos, anchor) ?? anchor
    }
    const chat = repo.chats?.find((c) => c.id === containerId)
    if (chat) return chat.workspaceId ?? null
  }
  return null
}

function chatModes(
  repos: readonly Repo[],
  subjects: readonly SidebarRow[],
  target: DropTargetRow,
): AllowedModes {
  if (target.kind !== 'chat' && target.kind !== 'folder' && target.kind !== 'branch') {
    return NO_MODES
  }
  const { chat } = entityStore.getState().byKind
  if (!subjects.some((s) => s.working || chat[s.id]?.data.working)) return ALL_MODES
  const projectId = projectOfRow(repos, target.id)
  const leaves = (containerId: string) => {
    const dest = workspaceOfContainer(repos, projectId, containerId)
    return dest !== null && subjects.every((s) => s.workspaceId !== dest)
  }
  const reorder = leaves(target.parentId ?? '')
  return { before: reorder, after: reorder, into: leaves(target.id) }
}

function allowedModes(subjects: readonly SidebarRow[], target: DropTargetRow): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  const ops = operationsState()
  if (subjects.some((s) => s.workspaceId && selectEntityBusy(ops, s.workspaceId))) return NO_MODES
  const allChats = subjects.every((s) => s.kind === 'chat')
  if (subjects.some((s) => s.working) && (target.inRecents || !allChats)) return NO_MODES
  if (subjects.some((s) => s.id === target.id)) return NO_MODES
  const kind = subjects[0].kind
  if (!target.inRecents && subjects.some((s) => s.kind !== kind)) return NO_MODES

  if (target.inRecents) {
    return subjects.every((s) => s.kind === 'chat' || s.kind === 'branch') ? ALL_MODES : NO_MODES
  }

  const repos = readSelect(selectRepos)

  if (kind === 'chat') return chatModes(repos, subjects, target)

  if (kind === 'folder') {
    const subjectHomeScopes = subjects.map((s) => resolveHomeRowScope(s.id))
    if (subjectHomeScopes.some((s) => s !== null)) {
      if (subjectHomeScopes.some((s) => s === null)) return NO_MODES
      const projectId = subjectHomeScopes[0]!.projectId
      if (subjectHomeScopes.some((s) => s!.projectId !== projectId)) return NO_MODES
      if (target.kind === 'branch') {
        return repoOfHeaderRow(repos, target.id)?.projectId === projectId ? REORDER_MODES : NO_MODES
      }
      const targetHome = resolveHomeRowScope(target.id)
      if (!targetHome || targetHome.projectId !== projectId) return NO_MODES
      return ALL_MODES
    }
  }

  if (kind === 'branch' && subjects.length === 1 && subjects[0].repoIcon) {
    const repoIcon = subjects[0].repoIcon
    if (target.kind === 'branch') {
      const targetRepo = repoOfHeaderRow(repos, target.id)
      return targetRepo && targetRepo.projectId === repoIcon.projectId ? REORDER_MODES : NO_MODES
    }
    const targetHome = resolveHomeRowScope(target.id)
    if (!targetHome || targetHome.projectId !== repoIcon.projectId) return NO_MODES
    const containerID = target.parentId ?? ''
    const containerIsRepoSafe =
      containerID === '' || resolveHomeRowScope(containerID)?.kind === 'folder'
    return {
      before: containerIsRepoSafe,
      after: containerIsRepoSafe,
      into: target.kind === 'folder',
    }
  }

  const targetScope =
    resolveRowRepo(repos, target.id) ??
    (target.kind === 'chat' ? resolveChatRepo(repos, target.id) : null)
  if (!targetScope) return NO_MODES

  for (const subject of subjects) {
    const subjectScope = resolveRowRepo(repos, subject.id)
    if (!subjectScope) return NO_MODES
    if (subjectScope.projectId !== targetScope.projectId) return NO_MODES
    if (subjectScope.repoId !== targetScope.repoId && subject.ownsWorktree) return NO_MODES
  }

  if (kind === 'folder') {
    const repo = repos.find((r) => r.id === targetScope.repoId)
    if (repo) {
      const subjectAnchor = nearestBranchAnchor(repo, subjects[0].id)
      if (subjects.some((s) => nearestBranchAnchor(repo, s.id) !== subjectAnchor)) return NO_MODES
      const reorderAnchor = nearestBranchAnchor(repo, target.parentId ?? '')
      const intoAnchor = nearestBranchAnchor(repo, target.id)
      return {
        before: subjectAnchor === reorderAnchor,
        after: subjectAnchor === reorderAnchor,
        into: subjectAnchor === intoAnchor,
      }
    }
  }

  const hasLocked = subjects.some((s) => isWorkspaceLockedInSidebar(repos, s.workspaceId))
  const hasNoOwner = subjects.some(
    (s) =>
      s.kind === 'branch' &&
      !!s.workspaceId &&
      owningChatIdOfWorkspace(repos, s.workspaceId) === null,
  )
  if (hasLocked || hasNoOwner) {
    const repo = repos.find((r) => r.id === targetScope.repoId)
    if (!repo) return NO_MODES
    const subjectAnchor = nearestBranchAnchor(repo, subjects[0].parentId ?? '')
    if (subjects.some((s) => nearestBranchAnchor(repo, s.parentId ?? '') !== subjectAnchor)) {
      return NO_MODES
    }
    const reorderOk = subjectAnchor === nearestBranchAnchor(repo, target.parentId ?? '')
    return {
      before: reorderOk,
      after: reorderOk && !resolvesToFirstChild(target, 'after'),
      into: target.kind === 'folder' && subjectAnchor === nearestBranchAnchor(repo, target.id),
    }
  }

  return ALL_MODES
}

function edgeBandFor(kind: string): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, DropTargetRow> = {
  allowedModes,
  edgeBandFor,
}
