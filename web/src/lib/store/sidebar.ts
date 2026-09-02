import { create } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import type { ChatType, FolderDTO, WorkspaceDTO } from '@/lib/types'
import {
  sortReposByOrder,
  toSidebarFolder,
  toSidebarStatus,
  toSidebarWorkspace,
} from '@/lib/store/build-repo-tree'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

// §5 7-value status union (drops the old 'agent-running' overlay — an agent in
// flight is now the separate `working` flag). locked / pr-conflicts / deleted
// are first-class statuses.
export type WorkspaceStatus =
  'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'

/**
 * A sidebar grouping folder. Purely a tree edge: it holds no worktree and no
 * branch, which is why a workspace joins one through its own `folderId` rather
 * than through `parentId` (that field stays the FORK parent — putting a folder
 * id in it silently breaks merge eligibility and the diff base).
 *
 * Typed here ahead of the backend that emits it (plan wave 1/2), so every
 * consumer tolerates its absence: a repo with no folders simply has none.
 */
export interface Folder {
  id: string
  repoId: string
  /** Owning folder id, or undefined/'' for a folder that sits at the repo root. */
  parentId?: string
  name: string
  order: number
}

/**
 * Stable empty folder list. Same rule as EMPTY_PROJECTS (lib/store/projects.ts):
 * a fresh `[]` per call makes a Zustand snapshot compare unstable and React
 * eventually throws "Maximum update depth exceeded" somewhere unrelated.
 * Read-only by convention — consumers only map/find/filter it.
 */
export const EMPTY_FOLDERS: Folder[] = []

/**
 * A conversation row of the sidebar tree — design spec §3.1's `chat` kind.
 *
 * `parentId` is the ONE edge (§3.2): another chat (this one is a thread of it),
 * a folder, or absent for the root of whatever workspace `workspaceId` names.
 * It is deliberately not split into "fork parent" and "folder edge" the way a
 * `Workspace`'s is — a chat has no branch, so it has no lineage a folder could
 * split.
 *
 * `workspaceId` is the workspace this chat OWNS, and absent is a real answer:
 * that is a BUBBLE, which borrows the ground of its nearest ancestor that owns
 * one. Both kinds are tree rows; neither is Recents-only.
 */
export interface Chat {
  id: string
  repoId: string
  /** This row's own kind. `branch` is the one the tree reads structurally: such
   *  a row IS the workspace it owns (a locked branch, a repo home), and it is
   *  that row's id — not the workspace's — that every placement is addressed
   *  by. Undefined only on a row cached before the daemon emitted the field. */
  type?: ChatType
  /** A chat id, a folder id, or undefined/'' for the root of `workspaceId`. */
  parentId?: string
  /** The workspace this chat owns, or undefined/'' for a bubble. */
  workspaceId?: string
  title: string
  /** Sibling sort key, SHARED with folders and workspaces at the same level. */
  order: number
}

/** Stable empty chat list — same rule as EMPTY_FOLDERS above. */
export const EMPTY_CHATS: Chat[] = []

export interface Workspace {
  id: string
  branch: string
  parentId?: string
  /** Owning sidebar folder (see `Folder`); absent when the workspace sits at the
   *  repo root. Backend-supplied; older frames simply omit it. */
  folderId?: string
  /** Sibling sort key within its level. Backend-supplied and dense; older frames
   *  omit it, in which case consumers fall back to arrival order. */
  order?: number
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
  /** True while an agent/long-running op is in flight (replaces 'agent-running'). */
  working?: boolean
  /** Derived from MergeEligibility — whether this ws can merge into its parent. */
  canMergeLocally?: boolean
  /** Predicted: merging this ws into its parent would conflict (blocks the merge). */
  mergeConflicts?: boolean
  /** Parent branch name when mergeable. */
  parentBranch?: string
  /** Open PR url, when the ws has one. */
  prUrl?: string
  /** Last background-operation error (e.g. a rejected reparent), surfaced to the user. */
  lastError?: string
  /** On-disk worktree directory, from the backend WorkspaceDTO. */
  localPath?: string
  /** Holder path for a placeholder workspace (no localPath; locked only when the
   *  branch is protected); drives the reconstructed reason and whether the
   *  Detach… action is offered. */
  heldByPath?: string
  /**
   * The CHAT row that owns this workspace, straight from the daemon
   * (`WorkspaceDTO.owningChatId`) — never guessed here. Every placement the
   * daemon accepts is addressed by a chat id, so this is what a create under
   * this workspace names as its parent (`handleCreate`).
   *
   * It is NOT this workspace's row id. For a locked branch or a repo home the
   * two coincide — `rows-from-repo.ts` draws those rows AS their owning
   * `branch` row — but a regular fork's owner is an ordinary conversation that
   * already renders as its own row beside it, so the workspace points at it
   * rather than becoming it.
   *
   * `''` when the daemon resolved none; absent on a row cached before the
   * field existed. Both mean "nothing to place by yet", and callers fall back
   * to the row they were handed.
   */
  owningChatId?: string
}

export interface Repo {
  id: string
  /** Owning project — used to derive the active project from a workspace route. */
  projectId?: string
  /** Dense index within its project's section. Backend-supplied; older frames
   *  omit it, in which case the repo sorts after the ordered ones in arrival
   *  order (the same rule buildSidebarTree applies to a workspace's). */
  order?: number
  name: string
  avatarLabel: string
  avatarColor: string
  avatarURL?: string
  workspaces: Workspace[]
  /** Grouping folders declared inside this repo. Optional: the backend that
   *  emits them lands in parallel, and an older frame carries none. */
  folders?: Folder[]
  /** Chat rows that resolve to this repo. Optional for the same reason
   *  `folders?` is, and one more: they arrive on their own reseed loop
   *  (app-sync-provider's per-repo tree subscription), so a repo whose seed has
   *  not landed yet — or one whose section is folded away, and therefore has no
   *  subscription open at all — legitimately has none, and every consumer must
   *  read that as "not yet", never as "this repo has no chats". */
  chats?: Chat[]
  /** Real id of the IsDefault workspace (the imported repo folder); the repo
   *  header opens it and the context pill labels it "default". Its branch is
   *  exposed as `defaultBranch` (below) so create-input validation can reserve
   *  the default branch. */
  defaultWorkspaceId?: string
  /** Branch name of the default (main-worktree) workspace, surfaced on the repo
   *  header. Used by create-input validation to reserve the default branch. */
  defaultBranch?: string
  /** Status of the default (main-worktree) workspace. It is not a tree row, so
   *  it has no Workspace entry to carry the status — consumers gating on the
   *  locked state (e.g. the file explorer's mutation menu items) read it from
   *  here. Default workspaces adopted from protected branches are 'locked'. */
  defaultWorkspaceStatus?: WorkspaceStatus
  /** `working` of the default (repo-home) workspace. It is not a tree row, so it
   *  has no Workspace entry to carry the flag — the repo header and the context
   *  pill read it from here to spin the repo's icon during an agent turn. */
  defaultWorking?: boolean
  /** On-disk root of the repo (RepoDTO.path). Used as the localPath fallback for
   *  the default workspace, which is not stored in the workspaces array. */
  localPath?: string
}

export type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'

/** A workspace row's new placement. Absent fields are left alone. */
export interface WorkspacePlacementWrite {
  id: string
  /** Sidebar folder, '' for the repo root. */
  folderId?: string
  /** Fork parent — only a drop that actually moves the fork edge sets this. */
  parentId?: string
  order?: number
}

/** A folder row's new placement. */
export interface FolderPlacementWrite {
  id: string
  /** A workspace id, another folder id, or '' for the repo root. */
  parentId?: string
  order?: number
}

/**
 * A repo's owning project and its index within that project's section.
 *
 * Both, never just the array position: the sidebar re-sorts by `order` whenever
 * a repo arrives on the entity stream, so an optimistic move that only
 * re-spliced the array would be undone by the next frame.
 */
export interface RepoPlacementWrite {
  id: string
  projectId: string
  order: number
}

/**
 * Everything one drop moves, as a single unit.
 *
 * A drop is optimistic, so the paint has to land before the request does —
 * including the siblings a move displaces, or the tree renumbers itself a frame
 * late and the row visibly jumps twice. Bundling them means a refusal snaps the
 * whole move back at once rather than un-picking it row by row.
 */
export interface SidebarPlacement {
  workspaces?: readonly WorkspacePlacementWrite[]
  folders?: readonly FolderPlacementWrite[]
  /** The destination project's repos, in their new order. */
  repos?: readonly RepoPlacementWrite[]
}

interface SidebarState {
  /**
   * Every repo of every VISIBLE project (see lib/store/project-visibility.ts) —
   * no longer just the active project's. Each carries its own `projectId`.
   */
  repos: Repo[]
  collapsedRepos: Set<string>
  collapsedWorkspaces: Set<string>
  /**
   * Projects the user has explicitly folded away. Same polarity as
   * `collapsedRepos`/`collapsedWorkspaces` above: unknown means OPEN.
   *
   * Showing every project at once is the feature — a fresh install must render
   * the whole sidebar, not a column of closed rows — so the default cannot be
   * "collapsed". Collapse is how the cost is bought back rather than avoided: a
   * project's repo + workspace WebSocket streams are subscribed only while it
   * is visible (see lib/store/project-visibility.ts), so folding one away is a
   * real teardown, not just a tidier list.
   *
   * The ACTIVE project stays visible regardless of this set — the app needs its
   * repo scope whether or not the row is folded.
   */
  collapsedProjects: Set<string>
  /**
   * Chats-panel rows the user has folded — folder ids and chat ids together,
   * since both kinds hold children. Same polarity as the three sets above.
   *
   * It lives HERE, in a store the workspace switch does not touch, rather than
   * in the panel: the panel is keyed by workspace id so a switch remounts it
   * (deliberately — a drag, a rename and a selection must not ride along), and
   * component state died with it, so every folder sprang open on the way back.
   */
  collapsedChatRows: Set<string>
  /** Persisted active tab so re-mounts don't reset it. */
  activeTab: SidebarTab
  addWorkspace: (repoId: string, wsId: string, branch: string, parentId?: string) => void
  deleteWorkspace: (wsId: string) => void
  // NOTE: there is deliberately no renameWorkspace here. A branch rename moves
  // the git branch AND the workspace's on-disk directory, so it belongs to the
  // daemon; the renamed WorkspaceDTO returns via applyWorkspaceDTO. A local
  // relabel is what made rename look like it worked while changing nothing.
  reparentWorkspace: (wsId: string, newParentId: string | undefined) => void
  /**
   * Apply a whole drop's worth of placement at once — see {@link SidebarPlacement}.
   * Paired with {@link capturePlacement}, which reads the same fields back
   * beforehand so a refusal is one call to undo.
   */
  applyPlacement: (placement: SidebarPlacement) => void
  toggleRepo: (repoId: string) => void
  toggleWorkspace: (wsId: string) => void
  toggleProject: (projectId: string) => void
  /** Fold a Chats-panel row away, or open it again. */
  toggleChatRow: (rowId: string) => void
  /**
   * Open a Chats-panel row, whatever it was.
   *
   * Its own call rather than a toggle, because the caller is putting something
   * INSIDE the row — filing into a box you cannot see is not what "+ in here"
   * means — and a toggle there would close a row that was already open.
   */
  openChatRow: (rowId: string) => void
  setActiveTab: (tab: SidebarTab) => void
  setRepos: (repos: Repo[]) => void
  /**
   * Merge freshly fetched repos into the tree without clobbering local state:
   * unknown repos are appended, and unknown workspaces are appended to repos
   * that already exist. Existing entries (with their hierarchy overlays and
   * optimistic edits) are left untouched.
   */
  mergeRepos: (repos: Repo[]) => void
  /**
   * §6 WS-driven workspace upsert: merge a complete WorkspaceDTO into its repo
   * by id (insert if new, replace fields if it already exists). A DTO whose
   * status is 'deleted' is a tombstone — it removes the workspace from the
   * tree. Replaces the old optimistic addWorkspace/deleteWorkspace BFS: the
   * backend owns the deletion set and emits one tombstone per removed id.
   */
  applyWorkspaceDTO: (dto: WorkspaceDTO) => void
  /**
   * §6 WS-driven folder upsert, the folder half of {@link applyWorkspaceDTO}:
   * merge a complete FolderDTO into its repo by id, or remove it when the frame
   * is a `status: 'deleted'` tombstone. Deleting a folder reparents its children
   * rather than cascading (a folder holds no worktrees), and the daemon emits
   * their moved DTOs on their own streams — so this only ever touches the one
   * row the frame names.
   */
  applyFolderDTO: (dto: FolderDTO) => void
}

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

function reconcileRepos(existing: Repo[], incoming: Repo[]): Repo[] {
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

/**
 * Read back exactly the fields a placement is about to overwrite, so the drop
 * that fired it can be undone in one call. The shape returned is the same
 * `SidebarPlacement` — an undo is just the previous placement re-applied.
 *
 * Repos come back in their CURRENT array order, which is what carries their
 * index within a project's section.
 */
export function capturePlacement(repos: Repo[], placement: SidebarPlacement): SidebarPlacement {
  const out: SidebarPlacement = {}

  if (placement.workspaces?.length) {
    const wanted = new Map(placement.workspaces.map((w) => [w.id, w]))
    const workspaces: WorkspacePlacementWrite[] = []
    for (const repo of repos) {
      for (const ws of repo.workspaces) {
        const patch = wanted.get(ws.id)
        if (!patch) continue
        workspaces.push({
          id: ws.id,
          ...(patch.folderId !== undefined && { folderId: ws.folderId ?? '' }),
          ...(patch.parentId !== undefined && { parentId: ws.parentId ?? '' }),
          ...(patch.order !== undefined && { order: ws.order ?? 0 }),
        })
      }
    }
    out.workspaces = workspaces
  }

  if (placement.folders?.length) {
    const wanted = new Map(placement.folders.map((f) => [f.id, f]))
    const folders: FolderPlacementWrite[] = []
    for (const repo of repos) {
      for (const folder of repo.folders ?? EMPTY_FOLDERS) {
        const patch = wanted.get(folder.id)
        if (!patch) continue
        folders.push({
          id: folder.id,
          ...(patch.parentId !== undefined && { parentId: folder.parentId ?? '' }),
          ...(patch.order !== undefined && { order: folder.order }),
        })
      }
    }
    out.folders = folders
  }

  if (placement.repos?.length) {
    const wanted = new Set(placement.repos.map((r) => r.id))
    const current: RepoPlacementWrite[] = []
    for (const repo of repos) {
      if (wanted.has(repo.id)) {
        current.push({ id: repo.id, projectId: repo.projectId ?? '', order: repo.order ?? 0 })
      }
    }
    out.repos = current
  }

  return out
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
function recordRepoScopes(repos: Repo[]): void {
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
      })
    }
  }
}

export function getInitialState() {
  return {
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
    collapsedChatRows: new Set<string>(),
    // The card's default-visible panel is Files (scrollLeft starts at 0 in
    // sidebar-carousel.tsx) — 'workspaces' stopped being a valid TABS entry
    // when the carousel narrowed to Files/Git (Task 15), and nothing else
    // ever corrects a cold-start default, so it must match the real default
    // panel or neither tab underlines on first load.
    activeTab: 'files' as SidebarTab,
  }
}

/** The four sets `sidebar-ui` holds; every one of them a fold-away list. */
type CollapseSets = Pick<
  SidebarState,
  'collapsedRepos' | 'collapsedWorkspaces' | 'collapsedProjects' | 'collapsedChatRows'
>

/**
 * Apply one collapse change and write the WHOLE record.
 *
 * One writer for all four sets, so a toggle can never persist its own list over
 * a record whose other three it forgot to carry — which four independent call
 * sites assembling the same four arrays is one edit away from at any time.
 */
function persist<K extends keyof CollapseSets>(
  state: CollapseSets,
  change: Pick<CollapseSets, K>,
): Pick<CollapseSets, K> {
  const next = { ...state, ...change }
  void saveSidebarUI({
    collapsedRepos: [...next.collapsedRepos],
    collapsedWorkspaces: [...next.collapsedWorkspaces],
    collapsedProjects: [...next.collapsedProjects],
    collapsedChatRows: [...next.collapsedChatRows],
  })
  return change
}

export const useSidebarStore = create<SidebarState>()((set) => ({
  ...getInitialState(),

  addWorkspace: (repoId, wsId, branch, parentId) =>
    set((s) => ({
      repos: s.repos.map((r) =>
        r.id !== repoId
          ? r
          : {
              ...r,
              workspaces: [
                ...r.workspaces,
                {
                  id: wsId,
                  branch,
                  ...(parentId !== undefined && { parentId }),
                  status: 'new' as WorkspaceStatus,
                  age: 'just now',
                },
              ],
            },
      ),
    })),

  deleteWorkspace: (wsId) =>
    set((s) => {
      // Collect the target and all non-locked descendants (shared with
      // getPostDeleteNavigationTarget below).
      const toDelete = collectDeletedIds(
        s.repos.flatMap((r) => r.workspaces),
        wsId,
      )
      return {
        repos: s.repos.map((r) => ({
          ...r,
          workspaces: r.workspaces.filter((w) => !toDelete.has(w.id)),
        })),
      }
    }),

  reparentWorkspace: (wsId, newParentId) =>
    set((s) => {
      const repo = s.repos.find((r) => r.workspaces.some((w) => w.id === wsId))
      if (!repo) return s
      // newParentId must exist in the same repo (or be undefined for root)
      if (newParentId !== undefined && !repo.workspaces.some((w) => w.id === newParentId)) return s
      // Reject cycles: walk up from newParentId; if we reach wsId it's a cycle
      if (newParentId !== undefined) {
        const wsMap = new Map(repo.workspaces.map((w) => [w.id, w]))
        const visited = new Set<string>()
        let cursor: string | undefined = newParentId
        while (cursor !== undefined) {
          if (cursor === wsId || visited.has(cursor)) return s
          visited.add(cursor)
          cursor = wsMap.get(cursor)?.parentId
        }
      }
      return {
        repos: s.repos.map((r) =>
          r.id !== repo.id
            ? r
            : {
                ...r,
                workspaces: r.workspaces.map((w) =>
                  w.id === wsId ? { ...w, parentId: newParentId } : w,
                ),
              },
        ),
      }
    }),

  applyPlacement: (placement) =>
    set((s) => {
      let repos = s.repos

      if (placement.workspaces?.length || placement.folders?.length) {
        const wsPatch = new Map((placement.workspaces ?? []).map((w) => [w.id, w]))
        const folderPatch = new Map((placement.folders ?? []).map((f) => [f.id, f]))
        repos = repos.map((repo) => {
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
        for (const repo of repos) {
          const write = patch.get(repo.id)
          if (!write) untouched.push(repo)
          else if (repo.projectId === write.projectId && repo.order === write.order)
            moved.push(repo)
          else moved.push({ ...repo, projectId: write.projectId, order: write.order })
        }
        moved.sort((a, b) => at.get(a.id)! - at.get(b.id)!)
        repos = [...untouched, ...moved]
      }

      return repos === s.repos ? s : { repos }
    }),

  toggleRepo: (repoId) =>
    set((s) => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      return persist(s, { collapsedRepos: next })
    }),

  toggleWorkspace: (wsId) =>
    set((s) => {
      const next = new Set(s.collapsedWorkspaces)
      next.has(wsId) ? next.delete(wsId) : next.add(wsId)
      return persist(s, { collapsedWorkspaces: next })
    }),

  toggleProject: (projectId) =>
    set((s) => {
      const next = new Set(s.collapsedProjects)
      next.has(projectId) ? next.delete(projectId) : next.add(projectId)
      return persist(s, { collapsedProjects: next })
    }),

  toggleChatRow: (rowId) =>
    set((s) => {
      const next = new Set(s.collapsedChatRows)
      next.has(rowId) ? next.delete(rowId) : next.add(rowId)
      return persist(s, { collapsedChatRows: next })
    }),

  openChatRow: (rowId) =>
    set((s) => {
      if (!s.collapsedChatRows.has(rowId)) return s
      const next = new Set(s.collapsedChatRows)
      next.delete(rowId)
      return persist(s, { collapsedChatRows: next })
    }),

  setActiveTab: (tab) => set({ activeTab: tab }),

  setRepos: (repos) => {
    recordRepoScopes(repos)
    set((s) => {
      const next = reconcileRepos(s.repos, repos)
      return next === s.repos ? s : { repos: next }
    })
  },

  mergeRepos: (incoming) =>
    set((s) => {
      recordRepoScopes(incoming)
      let changed = false
      let resort = false
      const next = [...s.repos]
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
        if (added.length > 0) {
          next[idx] = { ...existing, workspaces: [...existing.workspaces, ...added] }
          changed = true
        }
      }
      if (!changed) return s
      return { repos: resort ? sortReposByOrder(next) : next }
    }),

  applyWorkspaceDTO: (dto) =>
    set((s) => {
      if (dto.status !== 'deleted') {
        recordWorkspaceScope({
          projectId: dto.projectId,
          repoId: dto.repoId,
          wsId: dto.id,
          owningChatId: dto.owningChatId,
        })
      }
      // A 'deleted' tombstone removes the workspace from whichever repo holds
      // it — the backend owns the cascade, so we never BFS-remove locally.
      if (dto.status === 'deleted') {
        let changed = false
        const repos = s.repos.map((r) => {
          if (!r.workspaces.some((w) => w.id === dto.id)) return r
          changed = true
          return { ...r, workspaces: r.workspaces.filter((w) => w.id !== dto.id) }
        })
        return changed ? { repos } : s
      }

      const repoIdx = s.repos.findIndex((r) => r.id === dto.repoId)
      // The repo isn't in the tree yet (its RepoDTO seed hasn't landed, or the
      // repo belongs to a project that is not visible): drop the frame — the
      // per-repo seed/stream will deliver this workspace once the repo exists.
      if (repoIdx === -1) return s
      const repo = s.repos[repoIdx]

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
        }
        if (
          repo.defaultWorkspaceId === next.defaultWorkspaceId &&
          repo.defaultBranch === next.defaultBranch &&
          repo.defaultWorking === next.defaultWorking &&
          repo.defaultWorkspaceStatus === next.defaultWorkspaceStatus
        ) {
          return s
        }
        const repos = [...s.repos]
        repos[repoIdx] = next
        return { repos }
      }

      const ws = toSidebarWorkspace(dto)
      const existingIdx = repo.workspaces.findIndex((w) => w.id === dto.id)
      // A frame that changes nothing (a reconnect reseed, a duplicate push)
      // must not hand out a new `repos` array: every sidebar subscriber
      // re-derives on identity, so a no-op frame would still cost a render
      // pass across the whole tree.
      if (existingIdx !== -1 && isSameRow(repo.workspaces[existingIdx], ws)) return s
      const workspaces =
        existingIdx === -1
          ? [...repo.workspaces, ws]
          : repo.workspaces.map((w, i) => (i === existingIdx ? { ...w, ...ws } : w))
      const repos = [...s.repos]
      repos[repoIdx] = { ...repo, workspaces }
      return { repos }
    }),

  applyFolderDTO: (dto) =>
    set((s) => {
      // A tombstone removes the folder from whichever repo holds it. Its
      // children are NOT removed with it: the daemon reparents them and emits
      // their own frames, so cascading here would blank rows the backend kept.
      if (dto.status === 'deleted') {
        let changed = false
        const repos = s.repos.map((r) => {
          if (!r.folders?.some((f) => f.id === dto.id)) return r
          changed = true
          return { ...r, folders: r.folders.filter((f) => f.id !== dto.id) }
        })
        return changed ? { repos } : s
      }

      const repoIdx = s.repos.findIndex((r) => r.id === dto.repoId)
      // The repo isn't in the tree yet (its RepoDTO seed hasn't landed, or it
      // belongs to a project that is not visible): drop the frame — the folders
      // seed will deliver this folder once the repo exists.
      if (repoIdx === -1) return s
      const repo = s.repos[repoIdx]

      const folder = toSidebarFolder(dto)
      const existing = repo.folders ?? EMPTY_FOLDERS
      const existingIdx = existing.findIndex((f) => f.id === dto.id)
      // A frame that changes nothing (a reconnect reseed, a duplicate push) must
      // not hand out a new `repos` array: every sidebar subscriber re-derives on
      // identity, so a no-op frame would still cost a render pass across the
      // whole tree.
      if (existingIdx !== -1 && isSameRow(existing[existingIdx], folder)) return s
      // Merged, never replaced: this is one folder's frame, not the repo's set.
      // Appending is enough for placement — folders and workspaces share one
      // sibling space that buildSidebarTree sorts by `order`, so array position
      // carries nothing.
      const folders =
        existingIdx === -1
          ? [...existing, folder]
          : existing.map((f, i) => (i === existingIdx ? { ...f, ...folder } : f))
      const repos = [...s.repos]
      repos[repoIdx] = { ...repo, folders }
      return { repos }
    }),
}))

// Expose for test reset
;(useSidebarStore as unknown as { getInitialState: typeof getInitialState }).getInitialState =
  getInitialState
