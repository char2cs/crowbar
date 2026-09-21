import { toast } from '@/features/window/stores/toast-store'
import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { openChatIdInOwnView } from '@/features/panes/utils/pane-command-actions'
import { getPaneSplitDropOptions } from '@/features/panes/utils/pane-drop-zones'
import { isKnownChatId, resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { viewIdOf } from '@/features/panes/lib/pane-views'
import {
  levelWorkspaceOfBranchRow,
  resolveChatRepo,
  resolveRowRepo,
} from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import { watchReparent } from '@/components/sidebar/lib/reparent-settle'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import {
  applyChatPlacement,
  applyFolderPlacements,
  applyWorkspacePlacement,
} from '@/lib/store/applied-placement'
import { OwningChatNotRecordedError } from '@/lib/workspace-scope-url'
import { chatNotLoadedYet } from '@/components/sidebar/lib/row-actions'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { hideRowsForInFlightCreates } from '@/components/sidebar/lib/rows-from-pending'
import {
  applyPendingRemovals,
  descendantHiddenIds,
  renderedHiddenIds,
} from '@/components/layout/removal-plan'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import {
  placeWorkspace,
  placeFolder,
  placeHomeFolder,
  placeRepo,
} from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-api'
import { recentsForProject } from '@/components/sidebar/lib/recents-for-project'
import { resolveHomeOwnerId, rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { homeOwnerRowId, rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import {
  refreshRepoPlacements,
  useHomeTreeStore,
  applyHomeChatPlacement,
  applyHomeFolders,
  resolveHomeRowScope,
} from '@/lib/store/home-tree'
import { toSidebarFolder } from '@/lib/store/build-repo-tree'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
} from '@/features/workspace/lib/home-workspace-resolver'

/** One request `performSidebarDrop` fires, in the order it must land — the
 *  scoped-down `PlacementCall` from `drop-plan.ts` (git show 9ad89156), minus
 *  its `project`/`repo` variants (not reachable through this drag any more)
 *  and plus `chat` (a placement `drop-plan.ts` never had to express). */
type RowPlacementCall =
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
      // Undefined for a pure reorder within the folder's own CURRENT
      // container — see `planTreeRowDrop`'s own note on why that has to be
      // OMITTED, not merely re-sent unchanged.
      parentId?: string
      order: number
    }
  | { kind: 'chat'; workspaceId: string; chatId: string; parentId: string; order: number }
  | {
      kind: 'homeFolder'
      projectId: string
      folderId: string
      // Same omit-when-unchanged rule as the `'folder'` case above.
      parentId?: string
      order: number
    }
  | { kind: 'repoHome'; projectId: string; repoId: string; folderId: string; order: number }

/** A chat subject whose target lives in a different workspace — no endpoint
 *  can re-home a chat to another workspace's own aggregate (see `planChatDrop`). */
const UNSUPPORTED = 'unsupported' as const

function findNode(nodes: SidebarTreeNode[], id: string): SidebarTreeNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node
    const hit = findNode(node.children, id)
    if (hit) return hit
  }
  return undefined
}

/** The workspace whose fork-child space owns `containerId` — '' for a
 *  root-level container, which has no lineage to protect.
 *
 *  A folder's own `parentId` names whatever row it was filed under in ROW
 *  space, which for a workspace-owning row (a locked branch, or an ordinary
 *  fork once it has a chat) is that row's OWNING CHAT id, never the raw
 *  workspace id (`rows-from-repo.ts`, the same id `performCreateFolder`
 *  sends as `parentId` for exactly this reason). Checking only
 *  `workspaceIds.has(cursor)` missed that translation: a folder filed
 *  directly under such a row walked straight past its real anchor to bare
 *  root the moment `cursor` became that owning-chat id, because neither
 *  `workspaceIds` (raw workspace ids) nor `folderById` (folder ids) recognise
 *  it. Caught live: filing a fork into a folder nested under its own locked
 *  fork parent read as a cross-parent move and silently rebased the fork
 *  onto the repo's default checkout instead. */
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

/** Where a row lands among `rest`, relative to `targetId`. A stale/foreign
 *  `targetId` clamps to the end rather than refusing the drop. */
function insertIndex(rest: string[], targetId: string, mode: 'before' | 'after'): number {
  const at = rest.indexOf(targetId)
  if (at < 0) return rest.length
  return mode === 'after' ? at + 1 : at
}

/**
 * The repos as the tree draws them — removal-tray-filtered, like
 * `SidebarTreeSurface` feeds `rowsFromRepo`.
 *
 * Through `renderedHiddenIds`, not the tray store's raw `hiddenIds`: the two
 * differ by every LIVE entry's own primary row, which stays on screen
 * transformed in place (`RemovingSidebarRow`). Reading the raw set modelled a
 * tree the user is not looking at — a held FOLDER was gone from it with its
 * children re-homed to the folder's parent (`applyPendingRemovals`), while the
 * rendered tree still drew the folder with those children nested under it, so
 * every sibling index a drop computed during an eight-second hold was counted
 * over a different list than the one it was aimed at. The raw set is still the
 * base, so rows committed but not yet tombstoned stay hidden here exactly as
 * they are on screen.
 */
export function visibleRepos(): Repo[] {
  const tray = useRemovalTrayStore.getState()
  return applyPendingRemovals(
    useSidebarStore.getState().repos,
    renderedHiddenIds(tray.hiddenIds, tray.entries),
  )
}

/**
 * The exact rows one project's panel draws — `SpacePanel`'s own
 * `[...homeRows, ...repoRows]`. Every sibling index below is computed over
 * these, never over a per-workspace store (populated only while that
 * workspace's pane is mounted, so empty for the common repo-pane-open,
 * home-not-mounted drag — every reorder then wrote `order: 0`) or a hand-
 * merged chat/folder/repo list (which id'd a repo header by its workspace
 * while the row carries its owning chat's id, so the target was never found
 * and the index clamped to the end).
 */
export function renderedProjectRows(repos: readonly Repo[], projectId: string): SidebarRow[] {
  const homeWorkspaceId = getHomeWorkspaceId(projectId)
  const homeTree = homeWorkspaceId ? useHomeTreeStore.getState().trees[projectId] : undefined
  const hidden = descendantHiddenIds(useRemovalTrayStore.getState().entries)
  const rows = [
    ...(homeWorkspaceId && homeTree
      ? rowsFromHome(
          homeWorkspaceId,
          homeTree.chats.filter((c) => !hidden.has(c.id)),
          homeTree.folders.filter((f) => !hidden.has(f.id)),
          getHomeOwningChatId(projectId) ?? undefined,
        )
      : []),
    ...rowsForProject(repos, projectId),
  ]
  return [...hideRowsForInFlightCreates(rows, usePendingCreatesStore.getState().entries, projectId)]
}

/** `containerId`'s members in the order `SidebarTree` draws them, minus the lifted rows. */
function renderedSiblings(
  rows: readonly SidebarRow[],
  containerId: string,
  lifted: ReadonlySet<string>,
): string[] {
  return rows
    .filter((r) => (r.parentId ?? '') === containerId && !lifted.has(r.id))
    .sort(compareSidebarRows)
    .map((r) => r.id)
}

/** The project whose panel draws `wsId`'s rows — a repo workspace, or a project's home.
 *  Home is asked of the resolver (the source a Recents row's workspace is
 *  stamped from), across every project known, not only those with a tree loaded. */
function projectOfWorkspace(repos: readonly Repo[], wsId: string): string | null {
  const scope = resolveRowRepo(repos, wsId)
  if (scope?.projectId) return scope.projectId
  const projectIds = new Set<string>([
    ...(dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS).map((p) => p.id),
    ...Object.keys(useHomeTreeStore.getState().trees),
    ...repos.flatMap((r) => (r.projectId ? [r.projectId] : [])),
  ])
  for (const projectId of projectIds) {
    if (getHomeWorkspaceId(projectId) === wsId) return projectId
  }
  return null
}

/**
 * A `branch`/`folder` drop — adapts `drop-plan.ts`'s `planRowDrop` (git show
 * 9ad89156) to `SidebarRow`. Kept: the container/fork-lineage math
 * (`findNode`/`workspaceAnchor`/`resolvesToFirstChild`) and the
 * folder-edge-vs-fork-parent precedence. Dropped: `project`/`repo` subjects
 * (out of this drag's scope now — see `SIDEBAR_DROP_POLICY`) and the
 * `writes`/`capturePlacement` optimistic half (this plan's no-optimistic-
 * write convention; the WS-driven cache applies the daemon's own confirmed
 * placement, same as `performRenameWorkspaceBranch`).
 */
function planTreeRowDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const { collapsedChatRows } = useSidebarStore.getState()
  // The tree the user actually sees (and dropped into) is removal-tray-
  // filtered. Planning against the raw repos during an in-progress hold can
  // count a held (about-to-vanish) sibling in `rest`/`insertIndex`'s math,
  // or report a target as having children it no longer visibly has —
  // silently upgrading a plain reorder into a first-child reparent the drag
  // indicator never promised.
  const repos = visibleRepos()
  // `resolveRowRepo` is chat-blind by design; a CHAT sibling is still a
  // legal target for a branch/folder to reorder past (`allowedModes`).
  const scope =
    resolveRowRepo(repos, target.id) ??
    (target.kind === 'chat' ? resolveChatRepo(repos, target.id) : null)
  const repo = scope && repos.find((r) => r.id === scope.repoId)
  if (!repo || !scope?.projectId) return []
  const { repoId, projectId } = scope

  // Everything below this line is the WORKSPACE/FOLDER id space: `roots` is
  // built from `repo.workspaces`, and `placeWorkspace`/`reparentWorkspace`/
  // `placeFolder` all address rows by those ids. A branch row arrives here
  // carrying the id of the CHAT that owns it (`rows-from-repo.ts`), so it is
  // translated once, at this boundary, rather than at each of the six places
  // an id is compared below — the same move `resolveRowRepo` above makes.
  const wsSpace = (id: string) => workspaceIdOfBranchRow(repos, id) ?? id
  const targetId = wsSpace(target.id)

  const roots = buildSidebarTree(repo.workspaces, repo.folders ?? [])
  // `roots` holds no chats — it exists for the fork-lineage math only. What
  // the target holds, and which siblings a reorder indexes into, come from
  // the rows the tree draws: a level interleaves branches, folders AND
  // chats on one order, so an index over `roots` alone is off by every chat
  // sibling the row actually has.
  const repoRows = rowsFromRepo(repo)
  const hasChildren = repoRows.some((r) => r.parentId === target.id)
  // Keyed by the ROW id, not the translated one — the collapse set holds what
  // the tree draws (`SidebarTreeSurface`/`toggleChatRow`), which is the row.
  const expanded = !collapsedChatRows.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requestedRow = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
  const requested = requestedRow === '' ? '' : wsSpace(requestedRow)
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested
  const containerKind = containerNode?.kind ?? 'root'
  // The repo root's members hang off the header ROW (`rows-from-repo.ts`),
  // which is not a node in `roots` — hence the row-space translation.
  const homeRowId = repo.defaultWorkspaceId
    ? resolveHomeOwnerId(repo.defaultWorkspaceId, repo.defaultOwningChatId, repo.chats ?? [])
    : ''
  const containerRowId = containerId === '' ? homeRowId : requestedRow

  const lifted = new Set(subjects.map((s) => s.id))
  const rest = renderedSiblings(repoRows, containerRowId, lifted).map(wsSpace)
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, targetId, mode)

  const calls: RowPlacementCall[] = []
  subjects.forEach((row, i) => {
    const order = at + i
    const subject = { kind: row.kind, id: wsSpace(row.id) }
    // Nothing for `placeWorkspace`/`reparentWorkspace` to address — the
    // repo's own checkout is not a member of `repo.workspaces`.
    if (subject.id === repo.defaultWorkspaceId) return

    if (subject.kind === 'folder') {
      // Not a member of this repo at all — `SIDEBAR_DROP_POLICY` should
      // already have refused this, but nothing here should construct a call
      // for a folder id `placeFolder` can't recognise.
      if (!repo.folders?.some((f) => f.id === subject.id)) return
      // `requestedRow` is the drop's own destination container, in the SAME
      // untranslated ROW space `row.parentId` already carries — comparing
      // them directly (never the WORKSPACE-space `containerId` below) is what
      // proves this drop leaves the folder's container exactly as it is,
      // regardless of which id space that container happens to be named in.
      // A pure reorder like that must send NO `parentId` at all, never a
      // freshly-recomputed one that merely happens to name the same place:
      // the backend's own `MoveInput.ParentID *string` treats a nil field as
      // "leave this exactly where it is," which is the only shape guaranteed
      // never to trip `checkFolderContextMove`'s anchor walk (Go,
      // usecases/chat/internal/tree/validate.go) — re-sending a value this
      // planner derived independently compares ITS OWN anchor for that value
      // against the backend's own stored one instead of skipping the check
      // outright, and the two can disagree about a position both sides agree
      // the folder never actually left. Caught live: dragging a folder to the
      // top of its own sibling list 400'd with "a folder cannot move to a
      // different context" for a drop that changed no parent at all.
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
      // The reparent (202, rebases the fork in the background — see
      // `reparent-settle.ts`) has to genuinely LAND before the index it was
      // promised is asked for, not just answer 202; `fireRowPlacementCall`
      // waits on `watchReparent` between these two calls. The reparent
      // itself clears any folder edge, so the follow-up carries one back
      // only when landing inside one.
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

    // Landing directly under the current fork parent drops any folder edge —
    // but "dropped" is NOT bare '': a chat's Node.ParentID for a row sitting
    // directly under a locked branch (no deeper folder) is that branch's OWN
    // CHAT id (workspaceAnchorView/mergeHomeNode's convention throughout this
    // package), the exact value `owningChatIdOfWorkspace` resolves — while
    // '' names the true bare PROJECT root, a different level entirely.
    // Sending '' here filed the row at the project root instead of back
    // under its own fork parent, caught live: a fork dragged past a sibling
    // fork under the SAME locked branch silently left that branch's own
    // level, the panel never showing the reorder because the row was no
    // longer even one of that level's members. Written unconditionally
    // (never gated on the subject's OWN prior `ws.folderId`) since a
    // never-yet-touched or previously-corrupted row needs the SAME correction
    // a properly-placed sibling already carries, not a same-as-before no-op.
    // A CHATLESS branch is addressed by its own workspace id — the Node row
    // the daemon minted for it (`workspaceAnchorView`) — never bare ''.
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

  const rows = renderedProjectRows(repos, projectId)
  // 'after' an EXPANDED chat/folder is the slot before its first child — where
  // the line is drawn (`resolvesToFirstChild`). Never for a branch target: its
  // children are another workspace's level, which a chat cannot join.
  const firstChild =
    target.kind !== 'branch' &&
    resolvesToFirstChild(
      {
        kind: target.kind,
        id: target.id,
        expanded: !useSidebarStore.getState().collapsedChatRows.has(target.id),
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
 * `useHomeTreeStore` instead of a repo. Home never forks (no worktree to
 * clone), so nothing but a folder ever reaches this: the fork-lineage half
 * of `planTreeRowDrop` (reparent calls, `workspaceAnchor`) has no home
 * counterpart to mirror — only the container/sibling-index math does.
 * `homeWorkspaceId` roots the SAME id space `rows-from-home.ts` builds:
 * `buildSidebarTree([], ...)`, one root workspace that owns every chat and
 * folder, its own owning branch chat excluded from the sibling space it can
 * never itself be dropped relative to (it is not a row at all).
 */
function planHomeFolderDrop(
  projectId: string,
  homeWorkspaceId: string,
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const tree = useHomeTreeStore.getState().trees[projectId]
  if (!tree) return []
  // A repo header is a root sibling to reorder past, never a container.
  if (target.kind === 'branch' && mode === 'into') return []
  const homeRowId = homeOwnerRowId(
    homeWorkspaceId,
    tree.chats,
    getHomeOwningChatId(projectId) ?? undefined,
  )
  const roots = buildSidebarTree(
    [],
    tree.folders,
    tree.chats.filter((c) => c.id !== homeRowId),
  )
  const targetId = target.id
  // Siblings come from the rows the panel draws, not `roots`: at the home
  // root a repo's own header row is a real sibling `roots` never holds.
  const rows = renderedProjectRows(visibleRepos(), projectId)
  const hasChildren = rows.some((r) => r.parentId === target.id)
  const expanded = !useSidebarStore.getState().collapsedChatRows.has(target.id)
  // Only a home chat/folder (a node of `roots`) has a first-child slot: an
  // expanded repo header's children are its own branches, not home rows.
  const firstChild =
    mode !== 'into' &&
    findNode(roots, targetId) !== undefined &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requested = mode === 'into' || firstChild ? targetId : target.parentId || ''
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested

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
    renderedProjectRows(visibleRepos(), scope.projectId),
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

function planRowDrop(
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

async function fireRowPlacementCall(call: RowPlacementCall): Promise<void> {
  switch (call.kind) {
    case 'reparent': {
      // Baseline captured — and the subscription armed — BEFORE the request
      // goes out, so a frame that beats the 202 back is not missed. Only
      // once `wait()` resolves has the rebase genuinely landed (or been
      // refused, in which case it throws and the follow-up placement call
      // below never fires).
      const wait = watchReparent(call.wsId, call.parentId)
      await reparentWorkspace(call.wsId, call.parentId)
      return wait()
    }
    case 'workspace': {
      // A branch row's order is read off its WorkspaceDTO, which no placement
      // frame refreshes — the answer (the moved row and every shifted
      // folder/branch sibling) is applied directly.
      const { workspace, shifted } = await placeWorkspace(call.wsId, {
        ...(call.folderId !== undefined && { folderId: call.folderId }),
        order: call.order,
      })
      await applyWorkspacePlacement(call.wsId, workspace, shifted)
      return
    }
    case 'folder': {
      // Applied directly, same as row-actions.ts's folder writes: there is no
      // dedicated push channel for folders any more (Task 34), so this
      // response is the only confirmation the drop gets — written through to
      // the cache every rebuild reads before the store (applied-placement.ts).
      const { folder, shifted, shiftedRows } = await placeFolder(
        call.projectId,
        call.repoId,
        call.folderId,
        { ...(call.parentId !== undefined && { parentId: call.parentId }), order: call.order },
      )
      await applyFolderPlacements(call.repoId, [folder, ...shifted], shiftedRows)
      return
    }
    case 'homeFolder': {
      // {@link applyHomeFolders}, same direct-apply reasoning as the repo
      // `'folder'` case above — home has no dedicated folder push channel
      // either. No `bump`/`crowbar_folders` write to mirror: `useHomeTreeStore`
      // is its own cache, already updated by the apply itself.
      const { folder, shifted } = await placeHomeFolder(call.projectId, call.folderId, {
        ...(call.parentId !== undefined && { parentId: call.parentId }),
        order: call.order,
      })
      applyHomeFolders(call.projectId, [folder, ...shifted].map(toSidebarFolder))
      return
    }
    case 'chat': {
      // Reported live as "can't parent a chat into a folder": the request
      // succeeded every time (confirmed live — the daemon had the chat under
      // its new parent, survived a reload), but nothing on screen ever
      // moved. `setChatPlacement`'s own response used to be discarded here,
      // same shape the folder case above already fixed for exactly this
      // reason (Task 34: no dedicated push channel) — a chat reparent turns
      // out to be the same story: whatever broadcast this was meant to ride
      // does not confirm a folder-nested move in practice, so the row just
      // sat wherever it started until an unrelated full reseed happened to
      // catch it up. Applied directly now, the same "already the daemon's
      // own committed state, arriving over the request instead of a
      // stream" reasoning `performRenameFolder` documents for its own case.
      const { chat, shifted } = await setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
      })
      // A home chat lives in `useHomeTreeStore`, never in a repo: its own
      // response is applied there (the reseed its frame triggers is the
      // backstop); every other chat writes through the entity cache first.
      const home = resolveHomeRowScope(chat.id)
      if (home) {
        applyHomeChatPlacement(home.projectId, chat, shifted)
        return
      }
      await applyChatPlacement(chat, shifted)
      return
    }
    case 'repoHome':
      // The PATCH answers 204 — no confirmed row to apply directly, unlike
      // every other case above. Left waiting on the eventual `repos`
      // broadcast/re-read alone, a same-level reorder sat at its OLD spot for
      // several unindicated seconds (live-reported "can't reorder, but
      // nesting works" — nesting only LOOKS instant because the row vanishes
      // off the flat list the moment it lands inside a folder, masking the
      // same delay). Applied optimistically instead, with the exact
      // order/folderId the request is about to send; the re-read below still
      // reconciles it against the server's own decision (e.g. collateral
      // shifts to sibling repos) — a frame that races the Node projection
      // left the header where it started until reload.
      useSidebarStore.getState().applyPlacement({
        repos: [
          { id: call.repoId, projectId: call.projectId, folderId: call.folderId, order: call.order },
        ],
      })
      await placeRepo(call.projectId, call.repoId, { folderId: call.folderId, order: call.order })
      await refreshRepoPlacements(call.projectId)
      return
  }
}

/**
 * Middle of a Recents entry (spec §8.1: "into that view, opened"). `target`
 * is ensured live first — the active pane, if it wasn't already up anywhere
 * (the same "makes its own view" a click already does, §8.4) — and every
 * dragged chat is then merged beside it via `openChatIntoPane`'s own
 * dedup/plain-open/merge rules (never a re-implementation): dropping a chat
 * that is already up goes TO it, and a target already on screen grows
 * instead of reopening.
 *
 * Recents spans every active workspace in a project, so `target` and
 * `dragged` can easily belong to different ones. That used to be silently
 * refused; `resolveChatWorkspaceId` (features/panes/lib/pane-chat-workspace.ts)
 * now answers "which workspace does this chat belong to" for the render path
 * too, so it no longer has to be.
 */
function openRecentsEntryThenMerge(
  target: SidebarRow,
  targetChatId: string,
  dragged: readonly SidebarRow[],
): void {
  const findPaneFor = (chatId: string) =>
    Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === chatId)?.id

  let targetPaneId = findPaneFor(targetChatId)
  if (!targetPaneId) {
    // Literally "the same 'makes its own view' a click already does" — so it
    // calls the click's own function rather than re-deriving it from a drop
    // aimed at the active pane, which would have merged the target into
    // whatever was already there before the dragged rows even arrived.
    openChatInOwnPane(target)
    targetPaneId = findPaneFor(targetChatId)
  }
  if (!targetPaneId) return
  for (const subject of dragged) {
    // Dropped onto itself — nothing to merge. Compared by resolved CHAT id,
    // not row id: a Recents row whose chat owns a workspace is a `branch` row
    // (see `performRecentsDrop`), and the tree's copy of that same chat can
    // name it by workspace id instead.
    if (paneChatSubject(subject)?.chatId === targetChatId) continue
    openChatIntoPane(subject, targetPaneId, 'center')
  }
}

/**
 * Above/below a Recents entry (spec §8.1: "it moves to that slot") — the
 * drag-reorder `planChatDrop`'s own doc used to flag as real remaining work.
 * `subjectChatIds`/`targetChatId` are the CHAT ids the dragged rows name
 * (resolved by `performRecentsDrop`, which a Recents row's `kind` cannot
 * answer on its own — see there), but
 * `pane-slice.ts`'s persisted order is keyed by ENTRY id (a pane id, a
 * merged-set nanoid, or a bare chat id for a working-no-view row — never a
 * chat id on its own, since a SET's members share one slot); both are
 * resolved here against the project's own current, correctly-derived band
 * before being handed to `reorderRecentsEntry`. A SET dragged by one of its
 * members reorders the whole set, since the members have no independent
 * slot of their own.
 */
function reorderRecentsEntries(
  subjectChatIds: readonly string[],
  target: SidebarRow,
  targetChatId: string,
  mode: 'before' | 'after',
): void {
  const repos = useSidebarStore.getState().repos
  // A home chat's entry carries the project-HOME workspace, which no repo
  // claims — resolved home-aware, or every drop beside one was a silent no-op.
  const projectId =
    resolveHomeRowScope(targetChatId)?.projectId ??
    projectOfWorkspace(repos, target.workspaceId ?? '')
  if (!projectId) return
  const entries = recentsForProject(repos, projectId)
  const naturalOrder = entries.map((e) => e.id)
  const targetEntry = entries.find((e) => e.chatIds.includes(targetChatId))
  if (!targetEntry) return

  const moved = new Set<string>()
  for (const chatId of subjectChatIds) {
    const sourceEntry = entries.find((e) => e.chatIds.includes(chatId))
    if (!sourceEntry || sourceEntry.id === targetEntry.id || moved.has(sourceEntry.id)) continue
    moved.add(sourceEntry.id)
    windowPaneStore
      .getState()
      .paneActions.reorderRecentsEntry(sourceEntry.id, targetEntry.id, mode, naturalOrder)
  }
}

/**
 * A drop whose TARGET lives in a Recents band, not the tree — spec §8.1's
 * table gives this geometry a different meaning than the same drop over a
 * tree row: the middle opens the dragged chat(s) into that view, and
 * above/below reorders the band itself rather than writing a tree
 * placement.
 *
 * A Recents row is ALWAYS a chat — the band renders nothing else — but its
 * `kind` is not always `'chat'`: `recents-band.tsx` spreads `chatIconIndex`'s
 * fields onto the row so a chat that owns a workspace draws the tree's real
 * branch/lock/PR glyph, and `kind: 'branch'` rides along with them. Filtering
 * on `kind === 'chat'` here therefore dropped every such row on the floor,
 * both as a subject and as a target — and since a workspace-owning chat is the
 * common Recents row in a real install, that read as "Recents rows can't be
 * reordered" (live-reported). `paneChatSubject` is the resolver this file
 * already uses for the same question one gesture over (a row dropped onto a
 * pane): it answers with the chat a row names for both kinds, and null for a
 * folder or a workspace row with no chat behind it.
 */
function performRecentsDrop(subjects: SidebarRow[], target: SidebarRow, mode: DropMode): void {
  const targetChatId = paneChatSubject(target)?.chatId
  if (!targetChatId) return
  const subjectChatIds = subjects.flatMap((s) => {
    const chatId = paneChatSubject(s)?.chatId
    return chatId ? [chatId] : []
  })
  if (subjectChatIds.length === 0) return
  if (mode === 'into') {
    openRecentsEntryThenMerge(target, targetChatId, subjects)
    return
  }
  reorderRecentsEntries(subjectChatIds, target, targetChatId, mode)
}

/**
 * Commit a row-to-row drop — reorder/reparent a workspace or folder, place a
 * chat within its own workspace's chat tree, or — when the drop TARGET lives
 * in a Recents band (`targetInRecents`, threaded from `use-sidebar-drag.ts`'s
 * hit test) — open into that view or reorder the band itself
 * (`performRecentsDrop`, spec §8.1). `SIDEBAR_DROP_POLICY` has already
 * refused every other subject/target/mode combination, so this only ever has
 * to plan what it allows through.
 *
 * Calls fire in order, each awaited before the next: a multi-row move's
 * `order` is an index into the destination as it stands once the previous
 * call has landed, so firing them concurrently (`Promise.all`) could land
 * the move in an arbitrary arrangement (matches `sidebar-placement.ts`'s own
 * documented contract for `order`). No optimistic paint — the WS-driven
 * cache applies the daemon's confirmed state, same as every other row action
 * in this plan (`performRenameWorkspaceBranch`, `performCreateFolder`).
 */
export async function performSidebarDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
  targetInRecents = false,
): Promise<void> {
  if (targetInRecents) {
    performRecentsDrop(subjects, target, mode)
    return
  }
  try {
    const plan = planRowDrop(subjects, target, mode)
    if (plan === UNSUPPORTED) {
      toast.error('Moving a chat to a different workspace is not supported yet')
      return
    }
    for (const call of plan) {
      // react-doctor-disable-next-line async-await-in-loop -- accepted: sequential on purpose (see this function's own doc above) — Promise.all would race the writes.
      await fireRowPlacementCall(call)
    }
  } catch (err) {
    // A reparent is chat-addressed; a fork whose owner is not recorded yet
    // is "still loading", the same copy every other verb uses (row-actions.ts).
    if (err instanceof OwningChatNotRecordedError) {
      const row = subjects.find((s) => s.workspaceId === err.wsId)
      toast.error(chatNotLoadedYet('move', row?.branchName ?? row?.label))
      return
    }
    // `guardReparent` (Go, hierarchy/worktree.go) correctly refuses a
    // reparent onto a branch row the sidebar can show before its worktree is
    // ever actually checked out on disk — but the raw reason reaches here
    // verbatim inside `ReparentFailedError`'s message and used to hit the
    // user as literal Go usecase text ("usecases: parent branch is not yet
    // provisioned"), caught live. The right fix is refusing the drop before
    // it's attempted (`sidebar-drop-policy.ts`, once a workspace DTO carries
    // a provisioned signal) — this is the stopgap until it does.
    const message = err instanceof Error ? err.message : 'Failed to move row'
    toast.error(
      message.includes('parent branch is not yet provisioned')
        ? "That branch hasn't been checked out yet — try again once it has"
        : message,
    )
  }
}

/** The chat a row opens into a pane, and the workspace that chat belongs to. */
interface PaneChatSubject {
  chatId: string
  workspaceId: string
}

/**
 * What a dragged row means to the PANE system: one chat, and its owning
 * workspace — or null for a row that names no chat at all.
 *
 * Two row kinds resolve, and they are the two the user can actually drag onto
 * a pane:
 *
 *   - a **chat** row (the tree's bubbles, and every Recents row — a Recents
 *     SET renders each member as its own draggable chat row, so a drag from
 *     the band always grabs exactly one chat, which is what makes "one chat
 *     per pane" fall out for free);
 *   - a **branch** row — a WORKSPACE row, and already a chat row wearing a
 *     workspace's clothes: `rows-from-repo.ts` gives every workspace-owning
 *     row the id of the CHAT that owns its worktree. This used to be refused
 *     outright ("no pane has an 'open into' meaning for them yet"), which is
 *     exactly why dragging a workspace row onto the pane area did nothing.
 *     The chat is re-resolved through the sidebar's own tree
 *     (`owningChatIdOfWorkspace`, the same source the delete path reads)
 *     rather than trusting the row id blindly: a workspace whose owning chat
 *     has not arrived keeps its own workspace id as the row id, and opening
 *     THAT into a pane would point the pane at a chat that does not exist.
 *
 * A folder only folds and no 'workflow' row is produced anywhere yet, so both
 * stay null — a no-op rather than a guess at behaviour nothing has defined.
 */
function paneChatSubject(row: SidebarRow): PaneChatSubject | null {
  if (row.kind === 'chat') {
    const workspaceId = resolveChatWorkspaceId(row.id, row.workspaceId)
    return workspaceId ? { chatId: row.id, workspaceId } : null
  }
  if (row.kind !== 'branch' || !row.workspaceId) return null
  const owner =
    owningChatIdOfWorkspace(useSidebarStore.getState().repos, row.workspaceId) ??
    (isKnownChatId(row.id) ? row.id : null)
  if (!owner) return null
  return {
    chatId: owner,
    workspaceId: resolveChatWorkspaceId(owner, row.workspaceId) ?? row.workspaceId,
  }
}

/**
 * A row dropped onto a pane — spec §8.1/§8.2. Every drop here ADDS; nothing
 * this reaches for can remove a pane or evict a chat that is already showing
 * (the dwell-to-remove gesture this replaced is gone — Task 22).
 */
export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  for (const subject of subjects) {
    openChatIntoPane(subject, paneId, zone)
  }
}

/**
 * One chat, CLICKED — spec §8.4: "clicking a chat in the tree makes its own
 * view." A BRAND-NEW view, every time: a fresh `viewId` nothing else on
 * screen carries, holding this one chat.
 *
 * Its own rule, deliberately NOT `openChatIntoPane`'s. That one answers a
 * DROP, whose entire vocabulary is "into THIS pane, on THAT side" (§8.1) and
 * whose occupied-pane case is a MERGE: a split carved out of the target
 * pane's own share, tagged with the target's own view — "you asked for them
 * side by side, so you get them side by side" (§8.2). A click asks for
 * neither. Routing it through the drop with a synthetic `zone: 'center'` on
 * whichever pane happened to be active is exactly what made clicking a row
 * read as appending a chat to the view you were already in: measured live,
 * four clicks produced one Recents SET of four chats and a 50/25/12.5/12.5
 * cascade of splits nested inside the first pane. Merging two views is the
 * drag-and-drop gesture and only that.
 *
 *   - **already up anywhere → go TO it** (§8.2's "it never opens twice"),
 *     checked FIRST and against every pane, since the clicked row may be live
 *     in a pane other than the active one — including one in a view that is
 *     currently off screen, in which case `setActivePane` brings that whole
 *     view over. Same dedup pattern `openChatIntoPane` and
 *     `open-agent-chat.ts` both use. Its view is left exactly as it is —
 *     revealing a chat is a SWITCH, never a regrouping.
 *   - **an EMPTY pane on screen → it fills that one.** An empty pane is a
 *     fallback, not a view (see `pane-slice.ts`'s `dropEmptiedPanes`), so
 *     there is nothing there to preserve and nothing to open beside. The
 *     active pane first, so a click lands where the user is already looking.
 *   - **otherwise → a brand-new VIEW** (`addPane`), which takes the screen
 *     while the arrangement that was showing is parked whole — never
 *     `splitPane` on the active one, which would charge the view you were in
 *     for the view you asked for, and never a peer leaf tiled beside it,
 *     which is what "a new view" used to amount to and why two separately
 *     clicked chats still ended up side by side.
 *
 * `detachPaneToOwnView` covers the middle case, and is what makes "a brand-
 * new view" true of the whole function rather than only of the `addPane`
 * branch: a reused pane can be one member of a view somebody merged earlier,
 * and filling it in place would have silently added this chat to that group —
 * the same "it appended to what I was looking at" complaint, one level down.
 * It is a no-op for a pane that is already a view of its own, which is the
 * overwhelmingly common case.
 *
 * The chat and its workspace both come from `paneChatSubject`, so this no
 * longer refuses a row belonging to an off-screen workspace: the render path
 * resolves a pane's chat to its own workspace now (see
 * `features/panes/lib/pane-chat-workspace.ts`), which is the mechanism that
 * refusal stood in for. `space-content-actions.ts`'s click still NAVIGATES to
 * a row's workspace first — that is a routing decision about where the user
 * should be, and it is unaffected by this.
 */
export function openChatInOwnPane(subject: SidebarRow): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  // The reveal-or-vacant-or-new-view logic itself lives in
  // `openChatIdInOwnView` (pane-command-actions.ts) — shared with ⌘N's
  // new-chat command, which needs the exact same "open as its own view" rule
  // for a chat id it just minted rather than resolved from a dragged row.
  openChatIdInOwnView(resolved.chatId)
}

/**
 * One chat, DROPPED onto one pane — spec §8.1/§8.2. **The only gesture in
 * the app that MERGES two chats into one view.**
 *
 * The merge is a single fact, written once: `splitPane` carves the new pane
 * out of the target's own share of the window AND tags it with the target's
 * `viewId` (pane-slice.ts). Both halves of "one view" — the layout subtree
 * and the group membership — come from that one call, so they cannot drift.
 * This used to need a second, separate write (`groupIntoArrangement`, filing
 * both chat ids into a Recents entry) precisely because grouping had no
 * expression in the pane model at all; Recents now reads the group off the
 * panes, so a merge that lands in the layout is a merge Recents draws.
 *
 * §8.2's "it never opens twice" is a rule against DUPLICATION, not against
 * the merge. Read as a blanket refusal it made a split unreachable: once
 * every chat got a view of its own and only the showing view occupies the
 * screen, every chat the user had ever opened already had a pane — parked,
 * off screen, but a pane — so "already up → go TO it" fired for every
 * Recents row and every previously-clicked tree row, and a drop onto a pane
 * edge switched views instead of splitting. That is the "I can't create a
 * split" this function is the whole of.
 *
 * So the dedup is a MOVE, not a refusal: `mergePaneIntoView` lifts the pane
 * the chat is already in out of whatever view holds it and re-homes it as a
 * split of the target, inheriting the target's `viewId`. Still exactly one
 * pane per chat, still never a second `setPaneChat` — and the view it left
 * dissolves on its own when it held nothing else. Only two drops are still a
 * plain reveal: onto the pane already showing the chat (nothing to
 * rearrange), and onto the MIDDLE of an empty pane, where §8.4's "an empty
 * pane is a fallback, not a view" means there is nobody to be side by side
 * with in the first place.
 *
 * `paneChatSubject` resolves both the chat and its owning workspace, so a
 * row from a workspace other than the routed one lands like any other — the
 * render path resolves a pane's chat to its own workspace now
 * (`features/panes/lib/pane-chat-workspace.ts`), which is what the old
 * active-workspace refusal was standing in for.
 *
 * NOT reachable from a plain click any more — see `openChatInOwnPane` above
 * for why a click needs its own, merge-free rule.
 */
export function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  const { panes, paneActions, viewProjects } = windowPaneStore.getState()
  const chatId = resolved.chatId

  const target = panes[paneId]
  if (!target) return

  // LAW 4 (project-scoped panes §6.5): content never crosses a project.
  // Belt-and-braces — the geometry already makes this nearly unreachable,
  // since the only draggable rows are the active project's panel's and the
  // only pane tree on screen is the active project's — so an UNRESOLVABLE
  // project on either side is allowed through rather than refused: a sidebar
  // one frame behind must not break an ordinary same-project drop.
  const targetProject = viewProjects[viewIdOf(target)]
  const subjectProject = resolveChatProjectId(chatId, resolved.workspaceId)
  if (targetProject && subjectProject && targetProject !== subjectProject) {
    toast.error('That chat belongs to a different space')
    return
  }

  const existingPane = Object.values(panes).find((p) => p.chatId === chatId)

  // Middle of an EMPTY pane: a plain open, exactly where you dropped it. No
  // merge — an empty pane is a fallback, not a view, so there is nobody to
  // be side by side WITH; the pane keeps whatever view it already answers to.
  if (zone === 'center' && target.chatId === null) {
    if (existingPane) {
      paneActions.setActivePane(existingPane.id)
      return
    }
    paneActions.setPaneChat(paneId, chatId, null)
    paneActions.setActivePane(paneId)
    return
  }

  // Every other case is a MERGE — an edge always splits (spec §8.1: "into
  // this view, on that side"), and the middle of an already-occupied pane
  // can only ADD, never swap out what is already there (§8.2's rule 1 —
  // that silent swap is exactly the dwell-to-remove gesture's replacement),
  // so it falls back to the same split, defaulting to the right.
  //
  // "You asked for them side by side, so you get them side by side" (§8.2):
  // both branches below tag the arriving pane with `target`'s `viewId`, so
  // the two are one view from this moment — in the layout and in Recents
  // alike, which now reads its live rows off exactly that tag.
  const splitOptions = getPaneSplitDropOptions(zone === 'center' ? 'right' : zone)
  if (!splitOptions) return

  if (existingPane) {
    // Dropped onto the pane it is already in: the arrangement it is asking
    // for is the one it has.
    if (existingPane.id === paneId) {
      paneActions.setActivePane(paneId)
      return
    }
    paneActions.mergePaneIntoView(
      existingPane.id,
      paneId,
      splitOptions.direction,
      splitOptions.placement,
    )
    paneActions.setActivePane(existingPane.id)
    return
  }

  const newPaneId = paneActions.splitPane(
    paneId,
    splitOptions.direction,
    undefined,
    splitOptions.placement,
  )
  if (!newPaneId) return
  paneActions.setPaneChat(newPaneId, chatId, null)
  paneActions.setActivePane(newPaneId)
}
