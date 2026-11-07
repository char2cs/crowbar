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

// The one drop matrix for the unified sidebar tree. A SidebarRow has no repoId/projectId, so
// scope is resolved by matching a row id against each repo's home workspace, tree workspaces and
// folders. Runs from pointer-driven code, never render, so reading getState() here is allowed.

export interface RowScope {
  repoId: string
  projectId: string | undefined
}

// A drop target as the hit test rebuilds it. A Recents row and its tree copy share id and
// parentId, but spec §8.1 gives a drop on each a different meaning, so the DOM says which.
export type DropTargetRow = SidebarRow & { inRecents?: boolean }

// A row's owning repo/project, or null when nothing in the store claims the id (every rule then
// refuses). Chat-blind on purpose: a dragged chat is exempt from the same-repo rule (spec §8.3).
// Takes `repos` so one drag frame reads the store once, not once per subject.
export function resolveRowRepo(repos: readonly Repo[], rowId: string): RowScope | null {
  // A branch row is id'd by its owning chat; translate it to its workspace first, or the repo
  // home and every locked branch resolve to null and drops onto them silently do nothing.
  const id = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  const repo = repos.find(
    (r) =>
      r.defaultWorkspaceId === id ||
      r.workspaces.some((w) => w.id === id) ||
      r.folders?.some((f) => f.id === id),
  )
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

// A chat's owning repo/project, for a chat as drop TARGET only — resolving a dragged chat would
// defeat the §8.3 cross-repo exemption resolveRowRepo protects.
export function resolveChatRepo(repos: readonly Repo[], chatId: string): RowScope | null {
  const repo = repos.find((r) => r.chats?.some((c) => c.id === chatId))
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

// The repo whose own header row `rowId` is, else null. Never read row.repoIcon: a live drag's
// target is rebuilt from DOM attributes that don't include it. resolveRowRepo won't do either —
// it matches any row in the repo, not only its header.
export function repoOfHeaderRow(repos: readonly Repo[], rowId: string): Repo | null {
  const workspaceId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  return repos.find((r) => r.defaultWorkspaceId === workspaceId) ?? null
}

// The nearest workspace row at or above `rowId` in the drawn tree, mirroring the daemon's
// nearestWorkspaceAnchor: a folder may reorder within its branch's subtree but never jump to
// another. '' is the bare repo root, a real answer, not "not found".
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

// The project whose panel draws `rowId`: a home row, a repo's row, or a repo chat.
export function projectOfRow(repos: readonly Repo[], rowId: string): string | null {
  return (
    resolveHomeRowScope(rowId)?.projectId ??
    resolveRowRepo(repos, rowId)?.projectId ??
    resolveChatRepo(repos, rowId)?.projectId ??
    null
  )
}

// The workspace a chat filed under `containerId` runs in, as the daemon's checkChatContainer reads
// it: '' is the project home's root, a branch row or repo header its workspace, a folder the
// nearest branch above it (else the repo checkout or its home), a chat its own. Null: unknown id.
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

// A chat drop: anywhere a chat may hang, in any workspace. A working chat may only leave its
// workspace (the move waits for its turn boundary); within it the daemon refuses a running subtree.
function chatModes(
  repos: readonly Repo[],
  subjects: readonly SidebarRow[],
  target: DropTargetRow,
): AllowedModes {
  if (target.kind !== 'chat' && target.kind !== 'folder' && target.kind !== 'branch') {
    return NO_MODES
  }
  // A tree chat row is built with `working: false` (deriveRows): ask the entity store.
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

// Which of before/after/into this drag may do to this target. Only a chat crosses a project; across
// repos only rows owning no worktree move (§8.3). Busy rows refuse everything, working ones all but
// a chat's move out of its workspace, and a locked or ownerless branch keeps its fork anchor.
export function allowedModes(subjects: readonly SidebarRow[], target: DropTargetRow): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  const ops = operationsState()
  if (subjects.some((s) => s.workspaceId && selectEntityBusy(ops, s.workspaceId))) return NO_MODES
  const allChats = subjects.every((s) => s.kind === 'chat')
  if (subjects.some((s) => s.working) && (target.inRecents || !allChats)) return NO_MODES
  // Never drop a row onto itself.
  if (subjects.some((s) => s.id === target.id)) return NO_MODES
  // A mixed-kind selection is not a thing the sidebar can express; refuse
  // rather than guess which class wins.
  const kind = subjects[0].kind
  // Not for a Recents target: there every subject is a chat, whatever glyph
  // (`chat` or `branch`) its row wears, and a split carries both.
  if (!target.inRecents && subjects.some((s) => s.kind !== kind)) return NO_MODES

  // Recents is the project's view switcher (per-viewer order, spanning every workspace), so the
  // scope rules below don't apply: any chat — even one wearing a `branch` glyph — may open into
  // (middle) or reorder (edges) a Recents row (§8.1). A folder or workflow names no chat.
  if (target.inRecents) {
    return subjects.every((s) => s.kind === 'chat' || s.kind === 'branch') ? ALL_MODES : NO_MODES
  }

  // One getState() per drag frame, shared by every branch below.
  const repos = readSelect(selectRepos)

  // A chat is workspace-scoped, not repo-scoped, so the repo walk below doesn't apply. Gated on
  // the DRAGGED kind only: a folder or branch dropped on a chat is scoped further down.
  if (kind === 'chat') return chatModes(repos, subjects, target)

  // A project-home folder rides no repo, so resolveRowRepo can't see it; its only scope is the
  // project (one home, no fork lineage). Mixing home and repo-scoped subjects has no shared
  // container, so it refuses.
  if (kind === 'folder') {
    const subjectHomeScopes = subjects.map((s) => resolveHomeRowScope(s.id))
    if (subjectHomeScopes.some((s) => s !== null)) {
      if (subjectHomeScopes.some((s) => s === null)) return NO_MODES
      const projectId = subjectHomeScopes[0]!.projectId
      if (subjectHomeScopes.some((s) => s!.projectId !== projectId)) return NO_MODES
      // A repo's own header row shares the home root with a folder — a real
      // sibling to reorder past (its container is always root or a home
      // folder, its own placement invariant), never a container for one.
      if (target.kind === 'branch') {
        return repoOfHeaderRow(repos, target.id)?.projectId === projectId ? REORDER_MODES : NO_MODES
      }
      const targetHome = resolveHomeRowScope(target.id)
      if (!targetHome || targetHome.projectId !== projectId) return NO_MODES
      return ALL_MODES
    }
  }

  // A repo's own header row (`repoIcon`) is placed on the Repository aggregate, not by the repo
  // walk below. Its only legal moves are within its project's home: reorder past another header
  // or a home chat, reorder or nest at a home folder. Only ever dragged alone.
  if (kind === 'branch' && subjects.length === 1 && subjects[0].repoIcon) {
    const repoIcon = subjects[0].repoIcon
    if (target.kind === 'branch') {
      // Another header always sits in a container a repo may land in (its own placement
      // invariant), so no container check is needed.
      const targetRepo = repoOfHeaderRow(repos, target.id)
      return targetRepo && targetRepo.projectId === repoIcon.projectId ? REORDER_MODES : NO_MODES
    }
    const targetHome = resolveHomeRowScope(target.id)
    if (!targetHome || targetHome.projectId !== repoIcon.projectId) return NO_MODES
    // Reordering past target lands the repo in target's container, which must be home root or a
    // home folder, never a chat's thread space — even when target is itself a folder.
    // Nesting INTO target depends on target.kind alone.
    const containerID = target.parentId ?? ''
    const containerIsRepoSafe =
      containerID === '' || resolveHomeRowScope(containerID)?.kind === 'folder'
    return {
      before: containerIsRepoSafe,
      after: containerIsRepoSafe,
      into: target.kind === 'folder',
    }
  }

  // The subject is never a chat here, so a chat target is resolved by its own repo; otherwise a
  // folder or branch could never reorder past a repo chat sibling.
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

  // A folder may reorder only among siblings sharing its branch/root anchor (spec §2.6). Not
  // branches: their placement is fork lineage (Workspace.parentId), a different edge.
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
  // A fork with no owning chat recorded yet cannot re-parent either: that
  // verb is chat-addressed (workspace-scope-url.ts) and would only throw.
  const hasNoOwner = subjects.some(
    (s) =>
      s.kind === 'branch' &&
      !!s.workspaceId &&
      owningChatIdOfWorkspace(repos, s.workspaceId) === null,
  )
  if (hasLocked || hasNoOwner) {
    const repo = repos.find((r) => r.id === targetScope.repoId)
    if (!repo) return NO_MODES
    // Compare fork anchors, not raw parentIds: a container at the same anchor changes no
    // Workspace.parentId (planTreeRowDrop skips the reparent), so it is not the re-parent this
    // refuses — any folder at that anchor is a legal destination.
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

// The outer band reorders; the middle nests. Folders get the container band (filing is cheap and
// common); every other kind gets the heavy band, since nesting there re-parents a fork or makes a
// thread — heavier moves that deserve a harder-to-hit target.
export function edgeBandFor(kind: string): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, DropTargetRow> = {
  allowedModes,
  edgeBandFor,
}
