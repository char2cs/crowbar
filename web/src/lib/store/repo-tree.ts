import type { FolderDTO, WorkspaceDTO } from '@/lib/types'
import {
  sortReposByOrder,
  toSidebarFolder,
  toSidebarStatus,
  toSidebarWorkspace,
} from '@/lib/store/build-repo-tree'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import type { Chat, Folder, Repo, SidebarPlacement, Workspace } from '@/lib/store/sidebar'

// The sidebar's repo tree appliers: pure folds of daemon DTOs and confirmed
// placements into `Repo[]`. Each returns the SAME array when nothing changed,
// so a no-op frame costs no render. The row types live with the store
// (`lib/store/sidebar.ts`).

/**
 * Stable empty folder list. Same rule as EMPTY_PROJECTS (lib/store/projects.ts):
 * a fresh `[]` per call makes a Zustand snapshot compare unstable and React
 * eventually throws "Maximum update depth exceeded" somewhere unrelated.
 * Read-only by convention — consumers only map/find/filter it.
 */
export const EMPTY_FOLDERS: Folder[] = []

/** Stable empty chat list — same rule as EMPTY_FOLDERS above. */
export const EMPTY_CHATS: Chat[] = []

/**
 * Collect the workspace ids that deleting `wsId` removes: the target plus all
 * descendants, skipping locked subtrees — mirrors `deleteWorkspace` above.
 */
function collectDeletedIds(allWorkspaces: Workspace[], wsId: string): Set<string> {
  // Index once so the BFS below is O(n) instead of an array.find()/filter()
  // per queued id (this list is every workspace across every repo).
  const byId = new Map(allWorkspaces.map((w) => [w.id, w]))
  const childrenByParentId = new Map<string, Workspace[]>()
  for (const w of allWorkspaces) {
    if (!w.parentId) continue
    const siblings = childrenByParentId.get(w.parentId)
    if (siblings) siblings.push(w)
    else childrenByParentId.set(w.parentId, [w])
  }

  const toDelete = new Set<string>()
  const queue = [wsId]
  while (queue.length > 0) {
    const id = queue.shift()!
    if (toDelete.has(id)) continue
    const ws = byId.get(id)
    if (ws?.status === 'locked') continue
    toDelete.add(id)
    for (const child of childrenByParentId.get(id) ?? []) {
      queue.push(child.id)
    }
  }
  return toDelete
}

/**
 * Whether merging `incoming` (freshly built from a DTO) into `existing` would
 * change anything. Both applyWorkspaceDTO and applyFolderDTO merge with
 * `{...row, ...incoming}`, so only the keys `incoming` actually carries can move
 * the result — comparing those is enough, and it is what lets a no-op frame skip
 * the re-render.
 */
function isSameRow<T extends object>(existing: T, incoming: T): boolean {
  for (const key of Object.keys(incoming) as Array<keyof T>) {
    if (existing[key] !== incoming[key]) return false
  }
  return true
}

function isSameCompleteRow<T extends object>(existing: T, incoming: T): boolean {
  return (
    Object.keys(existing).length === Object.keys(incoming).length && isSameRow(existing, incoming)
  )
}

/**
 * Reuse rows that an authoritative cache rebuild recreated without changing.
 * Entity-cache reads deserialize every object afresh; handing those identities
 * straight to Zustand made a no-op seed look like a change to every row.
 */
function reconcileRows<T extends { id: string }>(existing: T[], incoming: T[]): T[] {
  const byId = new Map(existing.map((row) => [row.id, row]))
  const next = incoming.map((row) => {
    const current = byId.get(row.id)
    return current && isSameCompleteRow(current, row) ? current : row
  })
  return next.length === existing.length && next.every((row, index) => row === existing[index])
    ? existing
    : next
}

function sameRepoFields(existing: Repo, incoming: Repo): boolean {
  const keys = new Set([...Object.keys(existing), ...Object.keys(incoming)])
  keys.delete('workspaces')
  keys.delete('folders')
  keys.delete('chats')
  for (const key of keys as Set<keyof Repo>) {
    if (existing[key] !== incoming[key]) return false
  }
  return true
}

export function reconcileRepos(existing: Repo[], incoming: Repo[]): Repo[] {
  const byId = new Map(existing.map((repo) => [repo.id, repo]))
  const next = incoming.map((repo) => {
    const current = byId.get(repo.id)
    if (!current) return repo
    const workspaces = reconcileRows(current.workspaces, repo.workspaces)
    const folders =
      repo.folders === undefined
        ? undefined
        : reconcileRows(current.folders ?? EMPTY_FOLDERS, repo.folders)
    // Chats reconcile exactly as folders do, and for the same reason: their
    // reseed loop rebuilds every row from a fresh IndexedDB read, so handing
    // those identities straight to Zustand makes a no-op reseed look like a
    // change to every chat row in the tree.
    const chats =
      repo.chats === undefined ? undefined : reconcileRows(current.chats ?? EMPTY_CHATS, repo.chats)
    if (
      workspaces === current.workspaces &&
      folders === current.folders &&
      chats === current.chats &&
      sameRepoFields(current, repo)
    ) {
      return current
    }
    return {
      ...repo,
      workspaces,
      ...(folders === undefined ? {} : { folders }),
      ...(chats === undefined ? {} : { chats }),
    }
  })
  return next.length === existing.length && next.every((repo, index) => repo === existing[index])
    ? existing
    : next
}

/**
 * Whether `wsId` is a locked (protected-branch) workspace. Locked worktrees
 * refuse every daemon write (409 "workspace locked"), so mutation UI gates on
 * this. Checks BOTH id spaces a workspace can live in: the repo's tree rows
 * (`repo.workspaces`) AND the default (main-worktree) workspace, which is never
 * a tree row — it exists only as `repo.defaultWorkspaceId`, with its status
 * lifted onto `repo.defaultWorkspaceStatus` (adopted protected branches are
 * locked + default, so missing this branch un-gated every repo home).
 */
export function isWorkspaceLockedInSidebar(repos: Repo[], wsId: string | null): boolean {
  if (!wsId) return false
  for (const repo of repos) {
    if (repo.defaultWorkspaceId === wsId) return repo.defaultWorkspaceStatus === 'locked'
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws) return ws.status === 'locked'
  }
  return false
}

/** Merge one row's placement patch, leaving absent fields untouched. */
function withPlacement<T extends { order?: number }>(
  row: T,
  patch: { parentId?: string; folderId?: string; order?: number },
): T {
  return {
    ...row,
    ...(patch.folderId !== undefined && { folderId: patch.folderId }),
    ...(patch.parentId !== undefined && { parentId: patch.parentId }),
    ...(patch.order !== undefined && { order: patch.order }),
  }
}

/**
 * Where to navigate after deleting `wsId` while it (or one of its
 * descendants) is the active workspace: its parent if it survives, else the
 * repo's base (locked) workspace, else any surviving workspace in the repo,
 * else null (→ caller falls back to the projects page).
 */
export function getPostDeleteNavigationTarget(repos: Repo[], wsId: string): string | null {
  const repo = repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  if (!repo) return null
  const ws = repo.workspaces.find((w) => w.id === wsId)!
  const deleted = collectDeletedIds(
    repos.flatMap((r) => r.workspaces),
    wsId,
  )
  if (ws.parentId && !deleted.has(ws.parentId)) return ws.parentId
  const survivors = repo.workspaces.filter((w) => !deleted.has(w.id))
  const base = survivors.find((w) => w.status === 'locked')
  return (base ?? survivors[0])?.id ?? null
}

/**
 * Record the project/repo scope of every workspace a repo carries (including
 * the default workspace, which lives on the repo header rather than in the
 * tree). Workspace-scoped API calls (workspaceBase) throw on an unrecorded
 * scope, and the route only records the workspace you navigate to — so without
 * this, acting on a never-visited workspace (Retry/Detach… on a placeholder
 * row) failed before the request was sent. Repos without a projectId are
 * skipped: no scoped URL can be built for them anyway.
 */
export function recordRepoScopes(repos: Repo[]): void {
  for (const repo of repos) {
    if (!repo.projectId) continue
    for (const ws of repo.workspaces) {
      recordWorkspaceScope({
        projectId: repo.projectId,
        repoId: repo.id,
        wsId: ws.id,
        owningChatId: ws.owningChatId,
      })
    }
    if (repo.defaultWorkspaceId) {
      recordWorkspaceScope({
        projectId: repo.projectId,
        repoId: repo.id,
        wsId: repo.defaultWorkspaceId,
        owningChatId: repo.defaultOwningChatId || undefined,
      })
    }
  }
}

/** Apply a whole drop's worth of confirmed placement — see {@link SidebarPlacement}. */
export function applyPlacementTo(repos: Repo[], placement: SidebarPlacement): Repo[] {
  let next = repos

  if (placement.workspaces?.length || placement.folders?.length) {
    const wsPatch = new Map((placement.workspaces ?? []).map((w) => [w.id, w]))
    const folderPatch = new Map((placement.folders ?? []).map((f) => [f.id, f]))
    next = next.map((repo) => {
      const workspaces = repo.workspaces.map((w) => {
        const patch = wsPatch.get(w.id)
        return patch ? withPlacement(w, patch) : w
      })
      const folders = repo.folders?.map((f) => {
        const patch = folderPatch.get(f.id)
        return patch ? withPlacement(f, patch) : f
      })
      const workspacesChanged = workspaces.some((w, i) => w !== repo.workspaces[i])
      const foldersChanged = folders?.some((f, i) => f !== repo.folders?.[i]) ?? false
      if (!workspacesChanged && !foldersChanged) return repo
      return { ...repo, workspaces, ...(folders ? { folders } : {}) }
    })
  }

  if (placement.repos?.length) {
    // The flat `repos` array carries each repo's index within its project —
    // the sidebar buckets by `projectId` and keeps array order inside a
    // bucket — so a reorder is a re-splice of the destination's members.
    // Where the bucket lands in the flat array is irrelevant: group order
    // comes from the project list, not from here.
    //
    // `order` is written alongside the splice, not derived from it. The
    // array is re-sorted by that field whenever a repo arrives on the
    // entity stream, so a move that only changed positions would survive
    // exactly until the next frame.
    const at = new Map(placement.repos.map((r, i) => [r.id, i]))
    const patch = new Map(placement.repos.map((r) => [r.id, r]))
    const moved: Repo[] = []
    const untouched: Repo[] = []
    for (const repo of next) {
      const write = patch.get(repo.id)
      if (!write) {
        untouched.push(repo)
      } else if (
        repo.projectId === write.projectId &&
        repo.order === write.order &&
        (write.folderId === undefined || (repo.folderId ?? '') === write.folderId)
      ) {
        moved.push(repo)
      } else {
        moved.push({
          ...repo,
          projectId: write.projectId,
          order: write.order,
          ...(write.folderId !== undefined && { folderId: write.folderId }),
        })
      }
    }
    moved.sort((a, b) => at.get(a.id)! - at.get(b.id)!)
    next = [...untouched, ...moved]
  }

  return next
}

/**
 * Merge freshly fetched repos: unknown repos are appended (and the level
 * re-sorted), a known repo's own fields are overwritten, and its `workspaces`
 * are merged rather than replaced — a live single-repo frame carries `[]`
 * there, and replacing would wipe every workspace its chat stream populated.
 */
export function mergeReposInto(repos: Repo[], incoming: Repo[]): Repo[] {
  let changed = false
  let resort = false
  const next = [...repos]
  const byId = new Map(next.map((r, i) => [r.id, i]))
  for (const repo of incoming) {
    const idx = byId.get(repo.id)
    if (idx === undefined) {
      byId.set(repo.id, next.length)
      next.push(repo)
      changed = true
      // A repo arriving on the entity stream carries the index the user
      // dragged it to, and appending it would put it last regardless — a
      // reorder that looked like it worked, then quietly came undone on the
      // next frame. Placing it is a re-sort of the level it joins.
      resort = true
      continue
    }
    const existing = next[idx]
    const known = new Set(existing.workspaces.map((w) => w.id))
    const added = repo.workspaces.filter((w) => !known.has(w.id))
    // The incoming repo's OWN fields are authoritative — see this
    // action's own doc for why an already-known repo must not be left
    // untouched. `workspaces` is the one field kept separate: a live
    // single-repo frame always carries `[]` there, so replacing it
    // outright would wipe every workspace this repo's own chat-list
    // stream already populated.
    const merged = { ...existing, ...repo, workspaces: [...existing.workspaces, ...added] }
    if (merged.order !== existing.order || merged.folderId !== existing.folderId) {
      resort = true
    }
    next[idx] = merged
    changed = true
  }
  if (!changed) return repos
  return resort ? sortReposByOrder(next) : next
}

/**
 * Merge one complete WorkspaceDTO into its repo by id; a `status: 'deleted'`
 * tombstone removes it (the daemon owns the cascade and emits one per id).
 */
export function applyWorkspaceDTOTo(repos: Repo[], dto: WorkspaceDTO): Repo[] {
  // A 'deleted' tombstone removes the workspace from whichever repo holds
  // it — the backend owns the cascade, so we never BFS-remove locally.
  if (dto.status === 'deleted') {
    let changed = false
    const next = repos.map((r) => {
      if (!r.workspaces.some((w) => w.id === dto.id)) return r
      changed = true
      return { ...r, workspaces: r.workspaces.filter((w) => w.id !== dto.id) }
    })
    return changed ? next : repos
  }

  const repoIdx = repos.findIndex((r) => r.id === dto.repoId)
  // The repo isn't in the tree yet (its RepoDTO seed hasn't landed, or the
  // repo belongs to a project that is not visible): drop the frame — the
  // per-repo seed/stream will deliver this workspace once the repo exists.
  if (repoIdx === -1) return repos
  const repo = repos[repoIdx]

  // The default (main-worktree) workspace is never a tree ROW — it is
  // surfaced on the repo header via Repo.defaultWorkspaceId (see
  // toSidebarRepo). Never insert it as a row, but DO lift its live overlays
  // onto the header, exactly as toSidebarRepo does when the tree is rebuilt
  // wholesale: the repo avatar's agent spinner (defaultWorking) and the
  // lock gating (defaultWorkspaceStatus) read from there, and this is now
  // the only path a live frame takes.
  if (dto.isDefault) {
    const next: Repo = {
      ...repo,
      defaultWorkspaceId: dto.id,
      defaultBranch: dto.branch,
      defaultWorking: dto.working,
      defaultWorkspaceStatus: toSidebarStatus(dto),
      defaultOwningChatId: dto.owningChatId ?? '',
    }
    if (
      repo.defaultWorkspaceId === next.defaultWorkspaceId &&
      repo.defaultBranch === next.defaultBranch &&
      repo.defaultWorking === next.defaultWorking &&
      repo.defaultWorkspaceStatus === next.defaultWorkspaceStatus &&
      repo.defaultOwningChatId === next.defaultOwningChatId
    ) {
      return repos
    }
    return repos.map((r, i) => (i === repoIdx ? next : r))
  }

  const ws = toSidebarWorkspace(dto)
  const existingIdx = repo.workspaces.findIndex((w) => w.id === dto.id)
  // A frame that changes nothing (a reconnect reseed, a duplicate push)
  // must not hand out a new `repos` array: every sidebar subscriber
  // re-derives on identity, so a no-op frame would still cost a render
  // pass across the whole tree.
  if (existingIdx !== -1 && isSameRow(repo.workspaces[existingIdx], ws)) return repos
  const workspaces =
    existingIdx === -1
      ? [...repo.workspaces, ws]
      : repo.workspaces.map((w, i) => (i === existingIdx ? { ...w, ...ws } : w))
  return repos.map((r, i) => (i === repoIdx ? { ...repo, workspaces } : r))
}

/**
 * The folder half of {@link applyWorkspaceDTOTo}. A deleted folder's children
 * are reparented by the daemon, which emits their own frames — never cascaded.
 */
export function applyFolderDTOTo(repos: Repo[], dto: FolderDTO): Repo[] {
  // A tombstone removes the folder from whichever repo holds it. Its
  // children are NOT removed with it: the daemon reparents them and emits
  // their own frames, so cascading here would blank rows the backend kept.
  if (dto.status === 'deleted') {
    let changed = false
    const next = repos.map((r) => {
      if (!r.folders?.some((f) => f.id === dto.id)) return r
      changed = true
      return { ...r, folders: r.folders.filter((f) => f.id !== dto.id) }
    })
    return changed ? next : repos
  }

  const repoIdx = repos.findIndex((r) => r.id === dto.repoId)
  // The repo isn't in the tree yet (its RepoDTO seed hasn't landed, or it
  // belongs to a project that is not visible): drop the frame — the folders
  // seed will deliver this folder once the repo exists.
  if (repoIdx === -1) return repos
  const repo = repos[repoIdx]

  const folder = toSidebarFolder(dto)
  const existing = repo.folders ?? EMPTY_FOLDERS
  const existingIdx = existing.findIndex((f) => f.id === dto.id)
  // A frame that changes nothing (a reconnect reseed, a duplicate push) must
  // not hand out a new `repos` array: every sidebar subscriber re-derives on
  // identity, so a no-op frame would still cost a render pass across the
  // whole tree.
  if (existingIdx !== -1 && isSameRow(existing[existingIdx], folder)) return repos
  // Merged, never replaced: this is one folder's frame, not the repo's set.
  // Appending is enough for placement — folders and workspaces share one
  // sibling space that buildSidebarTree sorts by `order`, so array position
  // carries nothing.
  const folders =
    existingIdx === -1
      ? [...existing, folder]
      : existing.map((f, i) => (i === existingIdx ? { ...f, ...folder } : f))
  return repos.map((r, i) => (i === repoIdx ? { ...repo, folders } : r))
}
