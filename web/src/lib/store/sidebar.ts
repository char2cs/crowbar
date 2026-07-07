import { create } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import type { WorkspaceDTO } from '@/lib/types'
import { toSidebarWorkspace } from '@/lib/store/build-repo-tree'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

// §5 7-value status union (drops the old 'agent-running' overlay — an agent in
// flight is now the separate `working` flag). locked / pr-conflicts / deleted
// are first-class statuses.
export type WorkspaceStatus =
  | 'new'
  | 'locked'
  | 'pr-conflicts'
  | 'deleted'
  | 'pr-merged'
  | 'pr-open'
  | 'pr-closed'

export interface Workspace {
  id: string
  branch: string
  parentId?: string
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
  /** Holder path for a placeholder workspace (locked + no localPath); drives the
   *  reconstructed reason and whether the Detach… action is offered. */
  heldByPath?: string
}

export interface Repo {
  id: string
  /** Owning project — used to derive the active project from a workspace route. */
  projectId?: string
  name: string
  avatarLabel: string
  avatarColor: string
  avatarURL?: string
  workspaces: Workspace[]
  /** Real id of the IsDefault workspace (the imported repo folder); the repo
   *  header opens it and the context pill labels it "default". Its branch is
   *  exposed as `defaultBranch` (below) so create-input validation can reserve
   *  the default branch. */
  defaultWorkspaceId?: string
  /** Branch name of the default (main-worktree) workspace, surfaced on the repo
   *  header. Used by create-input validation to reserve the default branch. */
  defaultBranch?: string
  /** On-disk root of the repo (RepoDTO.path). Used as the localPath fallback for
   *  the default workspace, which is not stored in the workspaces array. */
  localPath?: string
}

export type SidebarTab = 'workspaces' | 'files' | 'git'

interface SidebarState {
  repos: Repo[]
  collapsedRepos: Set<string>
  collapsedWorkspaces: Set<string>
  /** Persisted active tab so re-mounts don't reset it. */
  activeTab: SidebarTab
  addWorkspace: (repoId: string, wsId: string, branch: string, parentId?: string) => void
  deleteWorkspace: (wsId: string) => void
  renameWorkspace: (wsId: string, branch: string) => void
  reparentWorkspace: (wsId: string, newParentId: string | undefined) => void
  toggleRepo: (repoId: string) => void
  toggleWorkspace: (wsId: string) => void
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
}

/**
 * Collect the workspace ids that deleting `wsId` removes: the target plus all
 * descendants, skipping locked subtrees — mirrors `deleteWorkspace` above.
 */
function collectDeletedIds(allWorkspaces: Workspace[], wsId: string): Set<string> {
  const toDelete = new Set<string>()
  const queue = [wsId]
  while (queue.length > 0) {
    const id = queue.shift()!
    if (toDelete.has(id)) continue
    const ws = allWorkspaces.find((w) => w.id === id)
    if (ws?.status === 'locked') continue
    toDelete.add(id)
    for (const child of allWorkspaces.filter((w) => w.parentId === id)) {
      queue.push(child.id)
    }
  }
  return toDelete
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
      recordWorkspaceScope({ projectId: repo.projectId, repoId: repo.id, wsId: ws.id })
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
    activeTab: 'workspaces' as SidebarTab,
  }
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
      // BFS to collect the target and all non-locked descendants
      const allWorkspaces = s.repos.flatMap((r) => r.workspaces)
      const toDelete = new Set<string>()
      const queue = [wsId]
      while (queue.length > 0) {
        const id = queue.shift()!
        const ws = allWorkspaces.find((w) => w.id === id)
        if (ws?.status === 'locked') continue
        toDelete.add(id)
        for (const child of allWorkspaces.filter((w) => w.parentId === id)) {
          queue.push(child.id)
        }
      }
      return {
        repos: s.repos.map((r) => ({
          ...r,
          workspaces: r.workspaces.filter((w) => !toDelete.has(w.id)),
        })),
      }
    }),

  renameWorkspace: (wsId, branch) =>
    set((s) => ({
      repos: s.repos.map((r) => ({
        ...r,
        workspaces: r.workspaces.map((w) => (w.id === wsId ? { ...w, branch } : w)),
      })),
    })),

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

  toggleRepo: (repoId) =>
    set((s) => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      void saveSidebarUI([...next], [...s.collapsedWorkspaces])
      return { collapsedRepos: next }
    }),

  toggleWorkspace: (wsId) =>
    set((s) => {
      const next = new Set(s.collapsedWorkspaces)
      next.has(wsId) ? next.delete(wsId) : next.add(wsId)
      void saveSidebarUI([...s.collapsedRepos], [...next])
      return { collapsedWorkspaces: next }
    }),

  setActiveTab: (tab) => set({ activeTab: tab }),

  setRepos: (repos) => {
    recordRepoScopes(repos)
    set({ repos })
  },

  mergeRepos: (incoming) =>
    set((s) => {
      recordRepoScopes(incoming)
      let changed = false
      const next = [...s.repos]
      const byId = new Map(next.map((r, i) => [r.id, i]))
      for (const repo of incoming) {
        const idx = byId.get(repo.id)
        if (idx === undefined) {
          byId.set(repo.id, next.length)
          next.push(repo)
          changed = true
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
      return changed ? { repos: next } : s
    }),

  applyWorkspaceDTO: (dto) =>
    set((s) => {
      if (dto.status !== 'deleted') {
        recordWorkspaceScope({ projectId: dto.projectId, repoId: dto.repoId, wsId: dto.id })
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

      // The default (main-worktree) workspace is never a tree row — it is
      // surfaced on the repo header via Repo.defaultWorkspaceId (see
      // toSidebarRepo). Skip it here so a stray live frame can't reintroduce it.
      if (dto.isDefault) return s

      const ws = toSidebarWorkspace(dto)
      const repoIdx = s.repos.findIndex((r) => r.id === dto.repoId)
      // The repo isn't in the tree yet (its RepoDTO seed hasn't landed): drop
      // the frame — the per-repo seed/stream will deliver this workspace once
      // the repo exists.
      if (repoIdx === -1) return s

      const repo = s.repos[repoIdx]
      const existingIdx = repo.workspaces.findIndex((w) => w.id === dto.id)
      const workspaces =
        existingIdx === -1
          ? [...repo.workspaces, ws]
          : repo.workspaces.map((w, i) => (i === existingIdx ? { ...w, ...ws } : w))
      const repos = [...s.repos]
      repos[repoIdx] = { ...repo, workspaces }
      return { repos }
    }),
}))

// Expose for test reset
;(useSidebarStore as unknown as { getInitialState: typeof getInitialState }).getInitialState =
  getInitialState
