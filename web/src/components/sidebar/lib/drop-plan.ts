import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
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

/**
 * A `chat` drop. A chat's placement lives on the `AgentChat` aggregate
 * itself, workspace-scoped (`setChatPlacement`, `@/features/agent/api/agent-
 * api`) — not `placeWorkspace`/`placeFolder`, which address `lib/store/
 * sidebar.ts`'s `Workspace`/`Folder` and know nothing about a chat.
 *
 * A chat may land relative to another chat, a folder (a real placement —
 * `Chat.parentId` is "a chat id, a folder id, or the root"), or PAST a
 * branch row it shares a level with (a repo header among project-home
 * rows, a locked branch among a repo's own root rows) — never INTO a branch,
 * which is not something a chat can become a thread of; `allowedModes`
 * refuses that mode and this is the backstop. The sibling index is computed
 * over the rows the panel draws (`renderedProjectRows`), the only source
 * that holds all three kinds on one order.
 */
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

/**
 * A project-home FOLDER drop — `planTreeRowDrop`'s sibling, scoped to
 * the project's home instead of a repo. Home never forks (no worktree to
 * clone), so nothing but a folder ever reaches this: the fork-lineage half
 * of `planTreeRowDrop` (reparent calls, `workspaceAnchor`) has no home
 * counterpart to mirror — only the container/sibling-index math does.
 * `homeWorkspaceId` roots the same id space the tree draws home rows in, its
 * owning chat excluded (it is the level, not a row in it).
 */
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
    // A home tree-drag subject is always a folder — home has no branch rows
    // to reparent (nothing to fork). Guarded rather than assumed:
    // `SIDEBAR_DROP_POLICY` should already have refused anything else, and
    // this is the same defensive check `planTreeRowDrop`'s own folder branch
    // makes before constructing a call `placeFolder` couldn't recognise.
    if (row.kind !== 'folder' || !tree.folders.some((f) => f.id === row.id)) return
    // Same reasoning as `planTreeRowDrop`'s own folder branch: a reorder that
    // leaves `row` in the exact container it already has must send no
    // `parentId` at all, not a freshly-recomputed one — see that function's
    // own note on why re-sending it can trip `checkFolderContextMove` for a
    // drop that changed no parent.
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

/**
 * A target's own project-home scope — the project whose home tree it is
 * either a member of (a chat/folder, via `resolveHomeRowScope`) or the repo
 * header row of (`target.repoIcon.projectId`). A repo's own entry is placed
 * relative to EITHER kind of sibling — a home chat/folder, or another repo's
 * header — and both have to resolve to "which project's home" the same way,
 * since `resolveHomeRowScope` only ever knows about chats and folders.
 */
function targetProjectHomeScope(target: SidebarRow): { projectId: string } | null {
  const homeScope = resolveHomeRowScope(target.id)
  if (homeScope) return { projectId: homeScope.projectId }
  if (target.repoIcon) return { projectId: target.repoIcon.projectId }
  return null
}

/**
 * A repo's own header row, dropped. `planTreeRowDrop`'s workspace branch has
 * nothing to address it with — it owns no fork lineage of its own to
 * reparent, and it is not a member of `repo.workspaces` for `placeWorkspace`
 * to touch (see that function's own early return) — because its placement
 * lives on a completely different aggregate, `domain.Repository` itself.
 *
 * Its one legal destination is project home: reordered among that project's
 * home chats, folders and other repos, or filed into one of that project's
 * home folders. Never into a chat (nothing for a repo to "thread" under) and
 * never into another repo (a repo is not a container) — `sidebar-drop-
 * policy.ts` refuses both before a drop ever reaches here, this is the
 * defensive backstop every other plan function in this file also keeps.
 */
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
  // A repo may only land at project-home root or inside a home FOLDER, never
  // inside a chat's own thread space — SIDEBAR_DROP_POLICY is the real gate
  // (it checks this exact container before a drop is ever offered), this is
  // the same defensive backstop every other plan function in this file keeps.
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
  // Home never forks, so a home tree-drag subject is always a folder — a
  // repo folder id can never collide with a home one (both come off the
  // daemon's own id space), so checking the first subject against every
  // visible project's home tree first is safe and cheap.
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
  // A repo's own header row — only ever dragged alone, never as part of a
  // multi-row selection: its placement lives on a whole different aggregate
  // (domain.Repository) from every other kind planned above, so it gets its
  // own plan rather than folding into planTreeRowDrop's workspace branch,
  // which has no way to address it at all.
  if (subjects.length === 1 && subjects[0].repoIcon) {
    return planRepoHomeDrop(subjects[0], target, mode)
  }
  return planTreeRowDrop(subjects, target, mode)
}
