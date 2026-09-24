import { toast } from '@/features/window/stores/toast-store'
import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import { isKnownChatId, resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
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
import {
  resolveHomeOwnerId,
  rowsFromRepo,
  rowRepoScope,
} from '@/components/sidebar/lib/rows-from-repo'
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
  return [
    ...hideRowsForInFlightCreates(
      rows,
      usePendingCreatesStore.getState().entries,
      projectId,
      rowRepoScope(repos),
    ),
  ]
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
          {
            id: call.repoId,
            projectId: call.projectId,
            folderId: call.folderId,
            order: call.order,
          },
        ],
      })
      await placeRepo(call.projectId, call.repoId, { folderId: call.folderId, order: call.order })
      await refreshRepoPlacements(call.projectId)
      return
  }
}

/**
 * Middle of a Recents row (spec §8.1: "into that view, opened"): every
 * dragged chat joins the target's view through `openChatIntoPane`'s own rules.
 */
function openRecentsEntryThenMerge(
  target: SidebarRow,
  targetChatId: string,
  dragged: readonly SidebarRow[],
): void {
  const findPaneFor = (chatId: string) =>
    chatPaneIndex(windowPaneStore.getState().panes).get(chatId)

  let targetPaneId = findPaneFor(targetChatId)
  if (!targetPaneId) {
    openChatInOwnPane(target)
    targetPaneId = findPaneFor(targetChatId)
  }
  if (!targetPaneId) return
  for (const subject of dragged) {
    // Compared by resolved CHAT id: a Recents row whose chat owns a workspace
    // is a `branch` row, and the tree's copy may name it by workspace id.
    if (paneChatSubject(subject)?.chatId === targetChatId) continue
    openChatIntoPane(subject, targetPaneId, 'center')
  }
}

/**
 * Above/below a Recents row (spec §8.1: "it moves to that slot"). A member
 * dragged out of a group leaves it first (`detachPane`); a whole row moves
 * within `viewOrder`.
 */
function reorderRecentsEntries(
  subjectChatIds: readonly string[],
  targetChatId: string,
  mode: 'before' | 'after',
): void {
  const viewOfChat = (chatId: string) => {
    const { panes } = windowPaneStore.getState()
    const paneId = chatPaneIndex(panes).get(chatId)
    return paneId ? panes[paneId]?.viewId : null
  }
  const targetViewId = viewOfChat(targetChatId)
  if (!targetViewId) return

  const moved = new Set<string>()
  for (const chatId of subjectChatIds) {
    const state = windowPaneStore.getState()
    const paneId = chatPaneIndex(state.panes).get(chatId)
    const sourceViewId = paneId ? state.panes[paneId]?.viewId : null
    if (!paneId || !sourceViewId || moved.has(sourceViewId)) continue
    if (viewChatIds(state, sourceViewId).length > 1) {
      state.paneActions.detachPane(paneId)
    } else if (sourceViewId === targetViewId) {
      continue
    }
    const viewId = viewOfChat(chatId)
    if (!viewId || viewId === targetViewId) continue
    moved.add(viewId)
    windowPaneStore.getState().paneActions.reorderView(viewId, targetViewId, mode)
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
  reorderRecentsEntries(subjectChatIds, targetChatId, mode)
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
 * One chat, CLICKED — spec §8.4: "clicking a chat makes its own view". A
 * click never merges; merging is the drag-and-drop gesture alone.
 */
export function openChatInOwnPane(subject: SidebarRow): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  windowPaneStore.getState().paneActions.openChat(resolved.chatId, {
    projectId: resolveChatProjectId(resolved.chatId, resolved.workspaceId) ?? undefined,
  })
}

/**
 * One chat, DROPPED onto one pane — spec §8.1/§8.2, the only gesture that
 * puts two chats in one view. An already-open chat is moved, never opened a
 * second time.
 */
export function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  const { panes, views, paneActions } = windowPaneStore.getState()
  const target = panes[paneId]
  if (!target) return

  // LAW 4: content never crosses a project. An unresolvable side is allowed
  // through — a sidebar one frame behind must not break a same-project drop.
  const targetProject = target.viewId ? views[target.viewId]?.projectId : undefined
  const subjectProject = resolveChatProjectId(resolved.chatId, resolved.workspaceId)
  if (targetProject && subjectProject && targetProject !== subjectProject) {
    toast.error('That chat belongs to a different space')
    return
  }
  paneActions.dropChatOnPane(resolved.chatId, paneId, zone)
}
