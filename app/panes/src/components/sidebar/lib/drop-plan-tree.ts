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

/** One request performSidebarDrop fires, in the order it must land. */
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
      // Omitted (not re-sent) for a reorder within the folder's current container.
      parentId?: string
      order: number
    }
  | {
      kind: 'chat'
      /** The chat's own workspace, which its placement is addressed by. */
      workspaceId: string
      chatId: string
      parentId: string
      order: number
      /** Set when the container is in another workspace: the chat moves there. */
      moveTo?: { workspaceId: string; label: string }
    }
  | {
      kind: 'homeFolder'
      projectId: string
      folderId: string
      // Same omit-when-unchanged rule as the `'folder'` case above.
      parentId?: string
      order: number
    }
  | { kind: 'repoHome'; projectId: string; repoId: string; folderId: string; order: number }

/** A container a branch or folder can land in, by its workspace-space id. */
function containerKindOf(repo: Repo, id: string): 'workspace' | 'folder' | undefined {
  if (repo.workspaces.some((w) => w.id === id)) return 'workspace'
  return repo.folders?.some((f) => f.id === id) ? 'folder' : undefined
}

// The workspace whose fork-child space owns `containerId`; '' for root, which has no lineage.
// A folder filed under a workspace-owning row names that row's owning-chat id, so each step also
// maps a chat id to its workspace; otherwise the walk overshoots to root and rebases the fork.
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

// A branch/folder drop: container and fork-lineage math, folder edge before fork parent. No
// optimistic write — the cache applies the daemon's confirmed placement.
export function planTreeRowDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): RowPlacementCall[] {
  const { collapsed } = treeUiStore.getState()
  // Plan against the removal-tray-filtered tree the user dropped into: a held sibling would skew
  // the index or turn a plain reorder into a first-child reparent the indicator never promised.
  const repos = visibleRepos()
  // `resolveRowRepo` is chat-blind by design; a CHAT sibling is still a
  // legal target for a branch/folder to reorder past (`allowedModes`).
  const scope =
    resolveRowRepo(repos, target.id) ??
    (target.kind === 'chat' ? resolveChatRepo(repos, target.id) : null)
  const repo = scope && repos.find((r) => r.id === scope.repoId)
  if (!repo || !scope?.projectId) return []
  const { repoId, projectId } = scope

  // Below is the WORKSPACE/FOLDER id space `placeWorkspace`/`reparentWorkspace`/`placeFolder`
  // address; a branch row carries its owning chat's id, translated once here.
  const wsSpace = (id: string) => workspaceIdOfBranchRow(repos, id) ?? id
  const targetId = wsSpace(target.id)

  // What the target holds, and which siblings a reorder indexes into, come from the rows
  // the tree draws: a level interleaves branches, folders AND chats on one order.
  const rows = renderedProjectRows(projectId)
  const hasChildren = rows.some((r) => r.parentId === target.id)
  // Keyed by the ROW id, not the translated one — the collapse set holds what
  // the tree draws (`SidebarTreeSurface`/`toggleChatRow`), which is the row.
  const expanded = !collapsed.has(target.id)
  const firstChild =
    mode !== 'into' &&
    resolvesToFirstChild({ kind: target.kind, id: target.id, expanded, hasChildren }, mode)
  const requestedRow = mode === 'into' || firstChild ? target.id : (target.parentId ?? '')
  const requested = requestedRow === '' ? '' : wsSpace(requestedRow)
  const found = requested === '' ? undefined : containerKindOf(repo, requested)
  const containerId = found === undefined ? '' : requested
  const containerKind = found ?? 'root'
  // The repo root's members hang off the header ROW, which names no workspace of
  // `repo.workspaces` — hence the row-space translation.
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
    // Nothing for `placeWorkspace`/`reparentWorkspace` to address — the
    // repo's own checkout is not a member of `repo.workspaces`.
    if (subject.id === repo.defaultWorkspaceId) return

    if (subject.kind === 'folder') {
      // Not a member of this repo at all — `SIDEBAR_DROP_POLICY` should
      // already have refused this, but nothing here should construct a call
      // for a folder id `placeFolder` can't recognise.
      if (!repo.folders?.some((f) => f.id === subject.id)) return
      // Compare in row space, which `row.parentId` uses: a reorder within the same container sends
      // NO parentId — the daemon treats nil as "stay put", while a re-derived value re-runs
      // checkFolderContextMove's anchor walk and can 400 a drop that changed no parent.
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
        throw new UserFacingError("That branch hasn't been checked out yet — try again once it has")
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

    // Landing directly under the current fork parent drops any folder edge, but the container is
    // that workspace's owning-chat id (a chatless branch: its workspace id), never '' — '' is the
    // project root. Written unconditionally so a misfiled row is corrected like its siblings.
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
