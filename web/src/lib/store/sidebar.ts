import { create } from 'zustand'
import type { ChatType, FolderDTO, WorkspaceDTO, WorkspaceProvisioning } from '@/lib/types'
import { forgetWorkspaceScope, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  applyFolderDTOTo,
  applyPlacementTo,
  applyWorkspaceDTOTo,
  mergeReposInto,
  reconcileRepos,
  recordRepoScopes,
} from '@/lib/store/repo-tree'
import {
  createSidebarUISlice,
  initialSidebarUIState,
  type SidebarUIState,
} from '@/lib/store/sidebar-ui'

// The sidebar store: the repo tree (row types below, appliers in repo-tree.ts)
// plus the sidebar's UI slice (sidebar-ui.ts).

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
  /** ISO creation time — the daemon's `order` tiebreak (see `compareByPlacement`). */
  createdAt?: string
}

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
  /** This row's own kind (see {@link ChatType}'s own doc) — never a signal
   *  for whether this chat owns a workspace any more (see
   *  {@link Chat.ownsWorktree}). Undefined only on a row cached before the
   *  daemon emitted the field. */
  type?: ChatType
  /** A chat id, a folder id, or undefined/'' for the root of `workspaceId`. */
  parentId?: string
  /** The workspace this chat RUNS IN — its own if it owns one, otherwise the
   *  one it borrows from an ancestor. Never proof of ownership on its own: a
   *  thread carries its parent's. See {@link Chat.ownsWorktree}. */
  workspaceId?: string
  /** Whether this row is the one that OWNS `workspaceId`'s worktree — i.e. this
   *  row is a workspace, not a bubble. Carried on the chat (see
   *  `ChatDTO.ownsWorktree`) so a row's KIND never depends on a separately
   *  streamed `Workspace` record having already landed. Undefined on a row
   *  cached before the field existed. */
  ownsWorktree?: boolean
  title: string
  /** Sibling sort key, SHARED with folders and workspaces at the same level. */
  order: number
  /** ISO creation time — the daemon's `order` tiebreak (see `compareByPlacement`). */
  createdAt?: string
}

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
  /** ISO creation time — the daemon's `order` tiebreak (see `compareByPlacement`). */
  createdAt?: string
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
  /** Holder path for a placeholder workspace; drives the reconstructed reason
   *  and whether the Detach… action is offered. */
  heldByPath?: string
  /** WorkspaceDTO.provisioning: whether this workspace has a checkout at all. */
  provisioning?: WorkspaceProvisioning
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
  /** Project-home folder this repo's own entry is filed under, undefined (or
   *  '') for the project's home root. Lets the repo header row interleave
   *  with the project's home chats/folders — see rows-from-repo.ts. */
  folderId?: string
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
  /** `WorkspaceDTO.owningChatId` of the default (repo-home) workspace,
   *  lifted here for the same reason `defaultBranch`/`defaultWorking` are:
   *  the default workspace is never a `Workspace` tree row, so there is no
   *  `Workspace.owningChatId` for `rows-from-repo.ts` to read directly. `''`
   *  when the daemon resolved none yet; absent on a row cached before the
   *  field existed — both mean "fall back to a chat that claims the row
   *  itself" (see `resolveHomeOwnerId`). */
  defaultOwningChatId?: string
  /** `working` of the default (repo-home) workspace. It is not a tree row, so it
   *  has no Workspace entry to carry the flag — the repo header and the context
   *  pill read it from here to spin the repo's icon during an agent turn. */
  defaultWorking?: boolean
  /** On-disk root of the repo (RepoDTO.path). Used as the localPath fallback for
   *  the default workspace, which is not stored in the workspaces array. */
  localPath?: string
  /** RepoDTO.lastError: why the last delete of this repo stopped. */
  deleteError?: string
}

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
  /** Project-home folder, '' for root. Undefined leaves it alone (see
   *  `Repo.folderId`'s own doc). */
  folderId?: string
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

interface RepoTreeState {
  /** Every repo of every VISIBLE project (lib/store/project-visibility.ts). */
  repos: Repo[]
  // No renameWorkspace: a branch rename moves the git branch and the worktree
  // directory, so it belongs to the daemon; the renamed DTO returns via
  // applyWorkspaceDTO.
  /** Apply a whole drop's worth of confirmed placement — see {@link SidebarPlacement}. */
  applyPlacement: (placement: SidebarPlacement) => void
  setRepos: (repos: Repo[]) => void
  /** See `mergeReposInto`. */
  mergeRepos: (repos: Repo[]) => void
  /** Drop a DELETED repo and forget its workspaces' scopes. */
  removeRepo: (repoId: string) => void
  /** §6 WS-driven workspace upsert/tombstone — see `applyWorkspaceDTOTo`. */
  applyWorkspaceDTO: (dto: WorkspaceDTO) => void
  /** §6 WS-driven folder upsert/tombstone — see `applyFolderDTOTo`. */
  applyFolderDTO: (dto: FolderDTO) => void
}

type SidebarState = RepoTreeState & SidebarUIState

export function getInitialState() {
  return { repos: [] as Repo[], ...initialSidebarUIState() }
}

export const useSidebarStore = create<SidebarState>()((set, get, api) => {
  /** Write `repos` only when an applier actually changed it. */
  const writeRepos = (apply: (repos: Repo[]) => Repo[]) =>
    set((s) => {
      const next = apply(s.repos)
      return next === s.repos ? s : { repos: next }
    })
  return {
    ...createSidebarUISlice(set, get, api),
    repos: [],
    applyPlacement: (placement) => writeRepos((repos) => applyPlacementTo(repos, placement)),
    setRepos: (incoming) => {
      recordRepoScopes(incoming)
      writeRepos((repos) => reconcileRepos(repos, incoming))
    },
    mergeRepos: (incoming) => {
      recordRepoScopes(incoming)
      writeRepos((repos) => mergeReposInto(repos, incoming))
    },
    removeRepo: (repoId) => {
      const repo = get().repos.find((r) => r.id === repoId)
      if (!repo) return
      for (const ws of repo.workspaces) forgetWorkspaceScope(ws.id)
      if (repo.defaultWorkspaceId) forgetWorkspaceScope(repo.defaultWorkspaceId)
      writeRepos((repos) => repos.filter((r) => r.id !== repoId))
    },
    applyWorkspaceDTO: (dto) => {
      if (dto.status === 'deleted') {
        forgetWorkspaceScope(dto.id)
      } else {
        recordWorkspaceScope({
          projectId: dto.projectId,
          repoId: dto.repoId,
          wsId: dto.id,
          owningChatId: dto.owningChatId,
        })
      }
      writeRepos((repos) => applyWorkspaceDTOTo(repos, dto))
    },
    applyFolderDTO: (dto) => writeRepos((repos) => applyFolderDTOTo(repos, dto)),
  }
})

// Expose for test reset
;(useSidebarStore as unknown as { getInitialState: typeof getInitialState }).getInitialState =
  getInitialState
