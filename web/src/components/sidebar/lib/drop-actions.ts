import { toast } from '@/features/window/stores/toast-store'
import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import {
  getActiveWorkspaceId,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getPaneSplitDropOptions } from '@/features/panes/utils/pane-drop-zones'
import { resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { watchReparent } from '@/components/sidebar/lib/reparent-settle'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { applyPendingRemovals } from '@/components/layout/removal-plan'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { buildChatTree } from '@/features/agent/tree/lib/chat-rows'
import { placeWorkspace, placeFolder } from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-api'
import { recentsForProject } from '@/components/sidebar/lib/recents-for-project'

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

    // Landing directly under the current fork parent drops any folder edge;
    // landing in one of its folders writes it. Independent of lineage, which
    // is unchanged on this branch.
    const directFolderId = containerKind === 'workspace' && ws.folderId ? '' : folderId
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
function planChatDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  // A repo/workspace folder (`rows-from-repo.ts`) and a chat folder
  // (`AgentChatFolder`) are different backend aggregates sharing one
  // `kind: 'folder'` tag; no row is ever both, so a chat has nothing real to
  // land on there. Refuse rather than guess.
  if (target.kind !== 'chat' || !target.workspaceId) return []
  const destWorkspaceId = target.workspaceId
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

function planRowDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] | typeof UNSUPPORTED {
  if (subjects.length === 0) return []
  return subjects[0].kind === 'chat'
    ? planChatDrop(subjects, target, mode)
    : planTreeRowDrop(subjects, target, mode)
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
    case 'chat':
      return setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
      }).then(() => undefined)
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
 * Carries the same off-screen-workspace limitation `openChatIntoPane`
 * already discloses on its own guard: a chat whose workspace isn't the one
 * currently active silently does nothing (no chatId->workspace resolution
 * exists in the render path yet). Recents can span every active workspace
 * in a project, so this is reachable in practice, not just hypothetically —
 * flagged rather than worked around here.
 */
function openRecentsEntryThenMerge(target: SidebarRow, dragged: readonly SidebarRow[]): void {
  const findPaneFor = (chatId: string) =>
    Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === chatId)?.id

  let targetPaneId = findPaneFor(target.id)
  if (!targetPaneId) {
    openChatIntoPane(target, windowPaneStore.getState().activePaneId, 'center')
    targetPaneId = findPaneFor(target.id)
  }
  if (!targetPaneId) return
  for (const subject of dragged) {
    if (subject.id === target.id) continue // dropped onto itself — nothing to merge
    if (subject.kind === 'chat') openChatIntoPane(subject, targetPaneId, 'center')
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
    toast.error(err instanceof Error ? err.message : 'Failed to move row')
  }
}

/**
 * A row dropped onto a pane — spec §8.1/§8.2. Every drop here ADDS; nothing
 * this reaches for can remove a pane or evict a chat that is already showing
 * (the dwell-to-remove gesture this replaced is gone — Task 22).
 *
 * Scoped to `kind === 'chat'` subjects. A branch/folder/workflow row has no
 * "open into a pane" meaning in this app today — a branch NAVIGATES to a
 * different workspace route entirely (`space-content-actions.ts`'s
 * `handleOpen`), a folder only folds, and no 'workflow' row is produced
 * anywhere yet (`SidebarRowKind` carries it for a future feature). Dropping
 * one of those kinds onto a pane is silently a no-op rather than a guess at
 * behavior nothing in the codebase has defined.
 */
export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  for (const subject of subjects) {
    if (subject.kind === 'chat') openChatIntoPane(subject, paneId, zone)
  }
}

/**
 * One chat, dropped OR CLICKED into one pane.
 *
 * §8.2: "dropping a chat that is already up goes TO it... it never opens
 * twice." Checked FIRST, before any zone/merge logic, and against every
 * pane in the chat's own workspace — not just `paneId` — because the row
 * dragged onto pane B might already be showing, live, in pane A. The
 * established dedup pattern (`open-agent-chat.ts`'s `openAgentChat`):
 * `Object.values(panes).find(p => p.chatId === chatId)`, reveal via
 * `setActivePane`, never a second `setPaneChat`.
 *
 * Exported for `space-content-actions.ts`'s `handleOpen` (spec §8.4:
 * "clicking a chat in the tree makes its own view") — a click is the same
 * "put this chat somewhere in a pane" question a drop asks, just answered
 * with a fixed target (the active pane, zone `'center'`) instead of one
 * resolved from where the pointer let go.
 */
export function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  if (!subject.workspaceId) return
  // Task 26 fix-round-1 (Critical 2): this refusal was removed once, on the
  // reasoning that panes/buffers are window-level now so there is no more
  // "wrong store" to mutate. That's true for the DATA side, but the RENDER
  // side was never rebuilt to match: a pane's "is this chat known" check
  // (AgentChatPane, via the AMBIENT WorkspaceStoreContext of whichever
  // WorkspaceView happens to render it) still resolves the chat against
  // whatever workspace is on screen, not the chat's real owning workspace —
  // no chatId->workspace lookup exists anywhere in the render path yet. A
  // chat from a different, off-screen workspace dropped in here would never
  // be found in the active workspace's agentChats.chats, so the pane renders
  // permanently blank (no CLI ever spawns) and — because setPaneChat persists
  // to IndexedDB — SURVIVES RELOAD. Restored until that resolution mechanism
  // (PaneGroup/chat carrying its own workspace identity through the render
  // path) is actually built; this guard is what stands in for it meanwhile.
  if (subject.workspaceId !== getActiveWorkspaceId()) return
  const { panes, paneActions } = windowPaneStore.getState()
  const chatId = subject.id

  const existingPane = Object.values(panes).find((p) => p.chatId === chatId)
  if (existingPane) {
    paneActions.setActivePane(existingPane.id)
    return
  }

  const target = panes[paneId]
  if (!target) return

  // Middle of an EMPTY pane: a plain open, exactly where you dropped it.
  if (zone === 'center' && target.chatId === null) {
    paneActions.setPaneChat(paneId, chatId, null)
    paneActions.setActivePane(paneId)
    return
  }

  // Every other case is a MERGE — an edge always splits (spec §8.1: "into
  // this view, on that side"), and the middle of an already-occupied pane
  // can only ADD, never swap out what is already there (§8.2's rule 1 —
  // that silent swap is exactly the dwell-to-remove gesture's replacement),
  // so it falls back to the same split, defaulting to the right.
  const splitOptions = getPaneSplitDropOptions(zone === 'center' ? 'right' : zone)
  if (!splitOptions) return
  const newPaneId = paneActions.splitPane(
    paneId,
    splitOptions.direction,
    undefined,
    splitOptions.placement,
  )
  if (!newPaneId) return
  paneActions.setPaneChat(newPaneId, chatId, null)
  paneActions.setActivePane(newPaneId)
  // "You asked for them side by side, so you get them side by side" (§8.2) —
  // only when there was something to merge WITH; splitting off an empty pane
  // opens one chat alone, nothing to group.
  if (target.chatId) paneActions.groupIntoArrangement([target.chatId, chatId])
}
