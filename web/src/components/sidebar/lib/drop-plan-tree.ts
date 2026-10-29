import { resolvesToFirstChild, type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveChatRepo, resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { resolveHomeOwnerId, rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { insertIndex, renderedSiblings, visibleRepos } from '@/components/sidebar/lib/rendered-rows'

/** One request `performSidebarDrop` fires, in the order it must land — the
 *  scoped-down `PlacementCall` from `drop-plan.ts` (git show 9ad89156), minus
 *  its `project`/`repo` variants (not reachable through this drag any more)
 *  and plus `chat` (a placement `drop-plan.ts` never had to express). */
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

export function findNode(nodes: SidebarTreeNode[], id: string): SidebarTreeNode | undefined {
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
export function planTreeRowDrop(
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
      // A fork parent must have a checkout to rebase onto; the daemon refuses
      // one without (guardReparent), so the drop is refused before any call.
      if (repo.workspaces.find((w) => w.id === nextFork)?.provisioning === 'placeholder') {
        throw new Error("That branch hasn't been checked out yet — try again once it has")
      }
      // `fireRowPlacementCall` places only once the reparent settled; the
      // reparent clears any folder edge, so the follow-up carries one back.
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
