import { toast } from '@/features/window/stores/toast-store'
import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isPaneEmpty } from '@/features/panes/stores/slices/pane-slice'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { getPaneSplitDropOptions } from '@/features/panes/utils/pane-drop-zones'
import { isKnownChatId, resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import { resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import { watchReparent } from '@/components/sidebar/lib/reparent-settle'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { applyPendingRemovals } from '@/components/layout/removal-plan'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { buildChatTree } from '@/features/agent/tree/lib/chat-rows'
import {
  placeWorkspace,
  placeFolder,
  placeHomeFolder,
  placeRepo,
} from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-api'
import { recentsForProject } from '@/components/sidebar/lib/recents-for-project'
import { resolveHomeOwnerId } from '@/components/sidebar/lib/rows-from-repo'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import { useHomeTreeStore, applyHomeFolders, resolveHomeRowScope } from '@/lib/store/home-tree'
import { toSidebarFolder } from '@/lib/store/build-repo-tree'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

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
      parentId: string
      order: number
    }
  | { kind: 'chat'; workspaceId: string; chatId: string; parentId: string; order: number }
  | { kind: 'homeFolder'; projectId: string; folderId: string; parentId: string; order: number }
  | { kind: 'repoHome'; projectId: string; repoId: string; folderId: string; order: number }

/** A chat subject whose target lives in a different workspace — no endpoint
 *  can re-home a chat to another workspace's own aggregate (see `planChatDrop`). */
const UNSUPPORTED = 'unsupported' as const

/** `buildChatTree`'s fold/search inputs don't affect `.siblings` at all (it
 *  is derived straight from the raw chat/folder set) — this call only ever
 *  wants that field, so every other input is this one stable empty. */
const EMPTY_ID_SET: ReadonlySet<string> = new Set()

function findNode(nodes: SidebarTreeNode[], id: string): SidebarTreeNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node
    const hit = findNode(node.children, id)
    if (hit) return hit
  }
  return undefined
}

/** Which sibling space a container owns. '' is the repo root. */
function membersOf(roots: SidebarTreeNode[], containerId: string): SidebarTreeNode[] {
  if (containerId === '') return roots
  return findNode(roots, containerId)?.children ?? []
}

/** The workspace whose fork-child space owns `containerId` — '' for a
 *  root-level container, which has no lineage to protect. */
function workspaceAnchor(repo: Repo, containerId: string): string {
  const workspaceIds = new Set(repo.workspaces.map((w) => w.id))
  const folderById = new Map((repo.folders ?? []).map((f) => [f.id, f]))
  const visited = new Set<string>()
  let cursor = containerId
  while (cursor !== '' && !visited.has(cursor)) {
    if (workspaceIds.has(cursor)) return cursor
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
 * A `branch`/`folder` drop — adapts `drop-plan.ts`'s `planRowDrop` (git show
 * 9ad89156) to `SidebarRow`. Kept: the container/fork-lineage math
 * (`findNode`/`membersOf`/`workspaceAnchor`/`resolvesToFirstChild`) and the
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
  const { repos: rawRepos, collapsedChatRows } = useSidebarStore.getState()
  // The tree the user actually sees (and dropped into) is removal-tray-
  // filtered — `SidebarTreeSurface` feeds `rowsFromRepo` this same
  // `applyPendingRemovals` view, not the raw store. Planning against the raw
  // repos during an in-progress hold can count a held (about-to-vanish)
  // sibling in `rest`/`insertIndex`'s math, or report a target as having
  // children it no longer visibly has — silently upgrading a plain reorder
  // into a first-child reparent the drag indicator never promised.
  const repos = applyPendingRemovals(rawRepos, useRemovalTrayStore.getState().hiddenIds)
  const scope = resolveRowRepo(repos, target.id)
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
  // The repo's own checkout (`rows-from-repo.ts`'s tree root) is a row but
  // never a node in `roots` — its rendered children ARE `roots` itself.
  const isHomeTarget = targetId === repo.defaultWorkspaceId
  const targetNode = isHomeTarget ? undefined : findNode(roots, targetId)
  const hasChildren = isHomeTarget ? roots.length > 0 : (targetNode?.children.length ?? 0) > 0
  // Keyed by the ROW id, not the translated one — the collapse set holds what
  // the tree draws (`SidebarTreeSurface`/`toggleChatRow`), which is the row.
  const expanded = !collapsedChatRows.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requested =
    mode === 'into' || firstChild ? targetId : target.parentId ? wsSpace(target.parentId) : ''
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested
  const containerKind = containerNode?.kind ?? 'root'

  const lifted = new Set(subjects.map((s) => wsSpace(s.id)))
  const rest = membersOf(roots, containerId)
    .map((n) => n.id)
    .filter((id) => !lifted.has(id))
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
      calls.push({
        kind: 'folder',
        projectId,
        repoId,
        folderId: subject.id,
        parentId: containerId,
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
    const directFolderId =
      containerKind === 'workspace' ? owningChatIdOfWorkspace(repos, containerId) ?? '' : folderId
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
 */
/**
 * A chat reordering PAST a BRANCH row (a repo's own header, a locked
 * branch, or an ordinary fork folded to look like one) — `allowedModes`
 * only ever calls this for `mode !== 'into'` (see sidebar-drop-policy.ts's
 * own doc on why "into" stays refused), so this only ever computes a
 * before/after index.
 *
 * The one thing `planChatDrop`'s own `buildChatTree`-based sibling read
 * cannot see is a BRANCH sibling: that read is scoped to one workspace's own
 * {chats, folders} (`getOrCreateWorkspaceStore(...).agentChats`), and a
 * branch row is neither — it is a Workspace or a Repository, never a Chat.
 * `rowsForProject`/`rowsFromHome` are reused here because they are the SAME
 * pipeline that already renders this exact combined level (repo header rows
 * interleaved with project-home chats/folders, and a repo's own locked
 * branches interleaved with its own top-level chats/folders) — recomputing
 * an equivalent sibling order any other way risks disagreeing with what is
 * actually on screen.
 *
 * Caught live: refusing this reorder outright used to be "can't put a chat
 * right at the bottom of the tree list" whenever a branch row happened to
 * occupy that position.
 */
function planChatDropOntoBranch(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  // `allowedModes` never returns `into: true` for a branch target — see
  // sidebar-drop-policy.ts's own doc — so this only ever reorders.
  if (mode === 'into') return []
  const repos = applyPendingRemovals(useSidebarStore.getState().repos, useRemovalTrayStore.getState().hiddenIds)
  const scope = resolveRowRepo(repos, target.id)
  if (!scope?.projectId) return []
  const projectId = scope.projectId
  const destWorkspaceId = subjects[0].workspaceId
  if (!destWorkspaceId) return []
  if (subjects.some((s) => s.workspaceId !== destWorkspaceId)) return UNSUPPORTED

  const containerId = target.parentId ?? ''
  const homeWorkspaceId = getHomeWorkspaceId(projectId)
  const homeTree = homeWorkspaceId ? useHomeTreeStore.getState().trees[projectId] : undefined
  const rows: SidebarRow[] = [
    ...(homeWorkspaceId && homeTree ? rowsFromHome(homeWorkspaceId, homeTree.chats, homeTree.folders) : []),
    ...rowsForProject(repos, projectId),
  ]
  const lifted = new Set(subjects.map((s) => s.id))
  const rest = rows
    .filter((r) => (r.parentId ?? '') === containerId && !lifted.has(r.id))
    .sort((a, b) => a.order - b.order)
    .map((r) => r.id)
  const at = insertIndex(rest, target.id, mode)

  return subjects.map((subject, i) => ({
    kind: 'chat' as const,
    workspaceId: destWorkspaceId,
    chatId: subject.id,
    parentId: containerId,
    order: at + i,
  }))
}

function planChatDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  if (target.kind === 'branch') return planChatDropOntoBranch(subjects, target, mode)
  // A chat CAN be filed into a folder — `Chat.parentId` is "a chat id, a
  // folder id, or the root" (lib/store/sidebar.ts's own doc), and a folder
  // groups every row kind in its level, chats included (buildSidebarTree:
  // "a level interleaves folders, branches and chats"). This used to refuse
  // any folder target outright — the literal "can't group chats into a
  // folder" gap, caught live, not a backend limitation: the placement route
  // already accepts a folder id as `parentId`.
  if (target.kind !== 'chat' && target.kind !== 'folder') return []
  // A folder carries no `workspaceId` of its own (pure organisation, no
  // ground — rows-from-repo.ts) — the dragged chat's OWN workspace is the
  // only ground a drop onto one can mean, and every subject is already
  // required to share it (checked right below), so it stands in whenever the
  // target itself has none to offer.
  const destWorkspaceId = target.kind === 'chat' ? target.workspaceId : subjects[0].workspaceId
  if (!destWorkspaceId) return []
  // `setChatPlacement` is scoped to one workspace's own chat tree — there is
  // no field on it that re-homes a chat to a different workspace's aggregate.
  if (subjects.some((s) => s.workspaceId !== destWorkspaceId)) return UNSUPPORTED

  const { chats, folders } = getOrCreateWorkspaceStore(destWorkspaceId).getState().agentChats
  // The real sibling order — dense `order` ascending, folders sorted above
  // chats on a tie, chats newest-first below that (`compareSiblings`) — is
  // NOT a plain `[...chats, ...folders]` concat; that puts every folder
  // after every chat regardless of where either actually sits. `siblings`
  // is `chat-rows.ts`'s own already-built, already-documented source for
  // "what a drop indexes into... a dropped row lands among its REAL
  // siblings" — reused here rather than re-derived.
  const { siblings } = buildChatTree({
    chats,
    folders,
    collapsed: EMPTY_ID_SET,
    shown: EMPTY_ID_SET,
    foldedAway: EMPTY_ID_SET,
    query: '',
  })
  // This function is reached ONLY for a TREE target now — `performSidebarDrop`
  // below branches a Recents-sourced drop (`targetInRecents`) off to
  // `performRecentsDrop` before this is ever called, since a drop over a
  // Recents row means something completely different (§8.1: "into that view,
  // opened" / "it moves to that slot" — a live pane write and Recents' own
  // persisted order, neither of which is a tree placement at all). What
  // follows is exactly the tree's own "make the subject one of the target's
  // threads" placement, unconditionally — the ambiguity this comment used to
  // describe (Recents rows render at depth 0 with `parentId: null`, same as a
  // root-level tree bubble, so 'before'/'after' here used to silently mean
  // "move to the tree root" for a Recents drag) is gone along with the need
  // to guess: a TREE row genuinely at the root behaves the same as it always
  // did, and a Recents row never reaches this branch at all any more.
  const containerId = mode === 'into' ? target.id : (target.parentId ?? '')
  const lifted = new Set(subjects.map((s) => s.id))
  const rest = (siblings.get(containerId) ?? []).filter((id) => !lifted.has(id))
  const at = mode === 'into' ? rest.length : insertIndex(rest, target.id, mode)

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
  const homeRowId = resolveHomeOwnerId(homeWorkspaceId, undefined, tree.chats)
  const roots = buildSidebarTree(
    [],
    tree.folders,
    tree.chats.filter((c) => c.id !== homeRowId),
  )
  const targetId = target.id
  const targetNode = findNode(roots, targetId)
  const hasChildren = (targetNode?.children.length ?? 0) > 0
  const expanded = !useSidebarStore.getState().collapsedChatRows.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requested = mode === 'into' || firstChild ? targetId : target.parentId || ''
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested

  const lifted = new Set(subjects.map((s) => s.id))
  const rest = membersOf(roots, containerId)
    .map((n) => n.id)
    .filter((id) => !lifted.has(id))
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, targetId, mode)

  const calls: RowPlacementCall[] = []
  subjects.forEach((row, i) => {
    // A home tree-drag subject is always a folder — home has no branch rows
    // to reparent (nothing to fork). Guarded rather than assumed:
    // `SIDEBAR_DROP_POLICY` should already have refused anything else, and
    // this is the same defensive check `planTreeRowDrop`'s own folder branch
    // makes before constructing a call `placeFolder` couldn't recognise.
    if (row.kind !== 'folder' || !tree.folders.some((f) => f.id === row.id)) return
    calls.push({
      kind: 'homeFolder',
      projectId,
      folderId: row.id,
      parentId: containerId,
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
 * The sibling id order one project-home CONTAINER holds right now — every
 * home chat, home folder, and OTHER repo's header row filed under it — sorted
 * the same loose way `buildSidebarTree` sorts one level (own `order`, ties
 * broken by arrival), so `insertIndex` below can place a dragged repo among
 * them precisely.
 *
 * A merge across two aggregates (`domain.Chat` for the chats/folders,
 * `domain.Repository` for the repos) rather than one shared table, so this
 * reads both stores directly instead of reusing `buildSidebarTree` (which
 * only ever takes one workspace/folder/chat family at a time). `excludeRepoId`
 * drops the repo being dragged out of its own current slot before it is
 * re-inserted, same as `lifted` does in every sibling computation above.
 */
function projectHomeContainerSiblings(
  projectId: string,
  containerId: string,
  homeWorkspaceId: string,
  excludeRepoId: string,
): string[] {
  const tree = useHomeTreeStore.getState().trees[projectId]
  const homeRowId = tree ? resolveHomeOwnerId(homeWorkspaceId, undefined, tree.chats) : null
  const entries: { id: string; order: number; arrival: number }[] = []
  let arrival = 0
  for (const c of tree?.chats ?? []) {
    if (c.id === homeRowId || (c.parentId || '') !== containerId) continue
    entries.push({ id: c.id, order: c.order ?? 0, arrival: arrival++ })
  }
  for (const f of tree?.folders ?? []) {
    if ((f.parentId || '') !== containerId) continue
    entries.push({ id: f.id, order: f.order ?? 0, arrival: arrival++ })
  }
  for (const r of useSidebarStore.getState().repos) {
    if (r.projectId !== projectId || (r.folderId || '') !== containerId) continue
    // Excluded by the REPO's own id, not its row id (`defaultWorkspaceId`) —
    // the two live in different id spaces, and comparing the wrong one left
    // the dragged repo counted among its own siblings, off-by-one-ing every
    // insert index past it.
    if (!r.defaultWorkspaceId || r.id === excludeRepoId) continue
    entries.push({ id: r.defaultWorkspaceId, order: r.order ?? 0, arrival: arrival++ })
  }
  entries.sort((a, b) => a.order - b.order || a.arrival - b.arrival)
  return entries.map((e) => e.id)
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
  const homeWorkspaceId = getHomeWorkspaceId(scope.projectId)
  if (!homeWorkspaceId) return []
  const containerId = mode === 'into' ? target.id : (target.parentId ?? '')
  // A repo may only land at project-home root or inside a home FOLDER, never
  // inside a chat's own thread space — SIDEBAR_DROP_POLICY is the real gate
  // (it checks this exact container before a drop is ever offered), this is
  // the same defensive backstop every other plan function in this file keeps.
  if (containerId !== '' && resolveHomeRowScope(containerId)?.kind !== 'folder') return []
  const rest = projectHomeContainerSiblings(
    scope.projectId,
    containerId,
    homeWorkspaceId,
    repoIcon.repoId,
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
    case 'workspace':
      return placeWorkspace(call.wsId, {
        ...(call.folderId !== undefined && { folderId: call.folderId }),
        order: call.order,
      })
    case 'folder': {
      // Applied directly, same as row-actions.ts's folder writes: there is no
      // dedicated push channel for folders any more (Task 34), so this
      // response is the only confirmation the drop gets. `bump` also writes
      // the `crowbar_folders` cache every tree rebuild reads from — without
      // it the move survives only until the next unrelated rebuild reverts
      // it (see row-actions.ts's performRenameFolder for the full story).
      const { folder, shifted } = await placeFolder(call.projectId, call.repoId, call.folderId, {
        parentId: call.parentId,
        order: call.order,
      })
      const apply = useSidebarStore.getState().applyFolderDTO
      apply(folder)
      shifted.forEach(apply)
      useFolderSignalStore.getState().bump(call.repoId)
      return
    }
    case 'homeFolder': {
      // {@link applyHomeFolders}, same direct-apply reasoning as the repo
      // `'folder'` case above — home has no dedicated folder push channel
      // either. No `bump`/`crowbar_folders` write to mirror: `useHomeTreeStore`
      // is its own cache, already updated by the apply itself.
      const { folder, shifted } = await placeHomeFolder(call.projectId, call.folderId, {
        parentId: call.parentId,
        order: call.order,
      })
      applyHomeFolders(call.projectId, [folder, ...shifted].map(toSidebarFolder))
      return
    }
    case 'chat':
      return setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
      }).then(() => undefined)
    case 'repoHome':
      // No direct-apply here, unlike the folder cases above: a repo's DTO
      // rides the same `repos` broadcast channel every OTHER repo write
      // already confirms through (rename, project move) — this is one more
      // field on that same write, not a new confirmation path to build.
      return placeRepo(call.projectId, call.repoId, { folderId: call.folderId, order: call.order })
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
function openRecentsEntryThenMerge(target: SidebarRow, dragged: readonly SidebarRow[]): void {
  const findPaneFor = (chatId: string) =>
    Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === chatId)?.id

  let targetPaneId = findPaneFor(target.id)
  if (!targetPaneId) {
    // Literally "the same 'makes its own view' a click already does" — so it
    // calls the click's own function rather than re-deriving it from a drop
    // aimed at the active pane, which would have merged the target into
    // whatever was already there before the dragged rows even arrived.
    openChatInOwnPane(target)
    targetPaneId = findPaneFor(target.id)
  }
  if (!targetPaneId) return
  for (const subject of dragged) {
    if (subject.id === target.id) continue // dropped onto itself — nothing to merge
    openChatIntoPane(subject, targetPaneId, 'center')
  }
}

/**
 * Above/below a Recents entry (spec §8.1: "it moves to that slot") — the
 * drag-reorder `planChatDrop`'s own doc used to flag as real remaining work.
 * `subjects`/`target` are chat ROWS (their `.id` is a chat id), but
 * `pane-slice.ts`'s persisted order is keyed by ENTRY id (a pane id, a
 * merged-set nanoid, or a bare chat id for a working-no-view row — never a
 * chat id on its own, since a SET's members share one slot); both are
 * resolved here against the project's own current, correctly-derived band
 * before being handed to `reorderRecentsEntry`. A SET dragged by one of its
 * members reorders the whole set, since the members have no independent
 * slot of their own.
 */
function reorderRecentsEntries(
  subjects: readonly SidebarRow[],
  target: SidebarRow,
  mode: 'before' | 'after',
): void {
  const repos = useSidebarStore.getState().repos
  const scope = resolveRowRepo(repos, target.workspaceId ?? '')
  if (!scope?.projectId) return
  const entries = recentsForProject(repos, scope.projectId)
  const naturalOrder = entries.map((e) => e.id)
  const targetEntry = entries.find((e) => e.chatIds.includes(target.id))
  if (!targetEntry) return

  const moved = new Set<string>()
  for (const subject of subjects) {
    const sourceEntry = entries.find((e) => e.chatIds.includes(subject.id))
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
 * placement. `SIDEBAR_DROP_POLICY` already refuses a mixed chat/non-chat
 * pairing, so both sides are guaranteed `kind: 'chat'` by the time this
 * runs; the checks below are a defensive backstop, not the real gate.
 */
function performRecentsDrop(subjects: SidebarRow[], target: SidebarRow, mode: DropMode): void {
  if (target.kind !== 'chat') return
  const chatSubjects = subjects.filter((s) => s.kind === 'chat')
  if (chatSubjects.length === 0) return
  if (mode === 'into') {
    openRecentsEntryThenMerge(target, chatSubjects)
    return
  }
  reorderRecentsEntries(chatSubjects, target, mode)
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
      await fireRowPlacementCall(call)
    }
  } catch (err) {
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
  const { panes, activePaneId, rootLayout, paneActions } = windowPaneStore.getState()
  const chatId = resolved.chatId

  const existingPane = Object.values(panes).find((p) => p.chatId === chatId)
  if (existingPane) {
    paneActions.setActivePane(existingPane.id)
    return
  }

  // Root layout only: a click never opens into the bottom panel, and
  // `activePaneId` can legitimately be it.
  const openPaneIds = getAllLeafIds(rootLayout)
  const vacant = (id: string) => isPaneEmpty(panes[id])
  const targetId =
    (openPaneIds.includes(activePaneId) && vacant(activePaneId) ? activePaneId : undefined) ??
    openPaneIds.find(vacant) ??
    paneActions.addPane()
  if (!targetId) return

  paneActions.detachPaneToOwnView(targetId)
  paneActions.setPaneChat(targetId, chatId, null)
  paneActions.setActivePane(targetId)
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
  const { panes, paneActions } = windowPaneStore.getState()
  const chatId = resolved.chatId

  const target = panes[paneId]
  if (!target) return
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
