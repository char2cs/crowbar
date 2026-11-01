import { create } from 'zustand'
import type { FolderDTO, WorkspaceDTO } from '@/lib/types'
import type { Repo } from '@/core/entities/repo-types'
import { forgetWorkspaceScope, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  applyFolderDTOTo,
  applyPlacementTo,
  applyWorkspaceDTOTo,
  mergeReposInto,
  reconcileRepos,
  recordRepoScopes,
} from '@/lib/store/repo-tree'
// The sidebar store: the repo tree (row types in core/entities/repo-types.ts, appliers in
// repo-tree.ts). Readers go through core/entities/select.ts.

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

export function getInitialState() {
  return { repos: [] as Repo[] }
}

export const useSidebarStore = create<RepoTreeState>()((set, get) => {
  /** Write `repos` only when an applier actually changed it. */
  const writeRepos = (apply: (repos: Repo[]) => Repo[]) =>
    set((s) => {
      const next = apply(s.repos)
      return next === s.repos ? s : { repos: next }
    })
  return {
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
