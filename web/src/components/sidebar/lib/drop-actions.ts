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
import { watchReparent } from '@/components/sidebar/lib/reparent-settle'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { applyPendingRemovals } from '@/components/layout/removal-plan'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { buildChatTree } from '@/features/agent/tree/lib/chat-rows'
import { placeWorkspace, placeFolder } from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-api'

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

  const roots = buildSidebarTree(repo.workspaces, repo.folders ?? [])
  // The repo's own checkout (`rows-from-repo.ts`'s tree root) is a row but
  // never a node in `roots` — its rendered children ARE `roots` itself.
  const isHomeTarget = target.id === repo.defaultWorkspaceId
  const targetNode = isHomeTarget ? undefined : findNode(roots, target.id)
  const hasChildren = isHomeTarget ? roots.length > 0 : (targetNode?.children.length ?? 0) > 0
  const expanded = !collapsedChatRows.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requested = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
  const containerNode = requested === '' ? undefined : findNode(roots, requested)
  const containerId = requested !== '' && !containerNode ? '' : requested
  const containerKind = containerNode?.kind ?? 'root'

  const lifted = new Set(subjects.map((s) => s.id))
  const rest = membersOf(roots, containerId)
    .map((n) => n.id)
    .filter((id) => !lifted.has(id))
  const at = mode === 'into' ? rest.length : firstChild ? 0 : insertIndex(rest, target.id, mode)

  const calls: RowPlacementCall[] = []
  subjects.forEach((subject, i) => {
    const order = at + i
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
  // Recents renders every row at depth 0 with no parentage (spec §5.1) —
  // `target.parentId` is always `null` there, so 'before'/'after' can only
  // ever place at the workspace root; only 'into' (making the subject one of
  // the target's own threads) reaches a real container.
  //
  // WHAT THIS DOES AND DOES NOT MOVE, stated plainly: this writes the chat's
  // placement on its own workspace aggregate, which is the durable order every
  // chat-tree reader sorts by. It does NOT move the row's slot in the RECENTS
  // BAND, because that band has no order of its own to write to:
  // `deriveRecentsEntries` computes it from `dormantArrangements` (a
  // window-level "closed-but-idle views, remembered so the close is undoable"
  // ledger, in memory only) followed by pane order then working order, and a
  // live pane's row or a working-no-view row has no record in that ledger at
  // all. Giving §8.1's "above / below a Recents entry → it moves to that slot"
  // its own persistence means freezing the whole derived order into a store
  // whose documented meaning is undo-a-close, which is a design change, not a
  // fix. Left as real remaining work rather than half-built here.
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
      await reparentWorkspace(call.projectId, call.repoId, call.wsId, call.parentId)
      return wait()
    }
    case 'workspace':
      return placeWorkspace(call.projectId, call.repoId, call.wsId, {
        ...(call.folderId !== undefined && { folderId: call.folderId }),
        order: call.order,
      })
    case 'folder':
      return placeFolder(call.projectId, call.repoId, call.folderId, {
        parentId: call.parentId,
        order: call.order,
      })
    case 'chat':
      return setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
      }).then(() => undefined)
  }
}

/**
 * Commit a row-to-row drop — reorder/reparent a workspace or folder, or
 * place a chat within its own workspace's chat tree. `SIDEBAR_DROP_POLICY`
 * has already refused every other subject/target/mode combination, so this
 * only ever has to plan what it allows through.
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
): Promise<void> {
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
 * One chat, dropped onto one pane.
 *
 * §8.2: "dropping a chat that is already up goes TO it... it never opens
 * twice." Checked FIRST, before any zone/merge logic, and against every
 * pane in the chat's own workspace — not just `paneId` — because the row
 * dragged onto pane B might already be showing, live, in pane A. The
 * established dedup pattern (`open-agent-chat.ts`'s `openAgentChat`):
 * `Object.values(panes).find(p => p.chatId === chatId)`, reveal via
 * `setActivePane`, never a second `setPaneChat`.
 */
function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
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
