import { useSidebarStore } from '@/lib/store/sidebar'
import {
  postWorkspace,
  deleteWorkspace as apiDeleteWorkspace,
  renameWorkspaceBranch,
  importBranches,
} from '@/lib/api'
import { placeFolder } from '@/lib/api/sidebar-placement'
import { toast } from '@/features/window/stores/toast-store'
import type { PendingCreate } from './workspace-tree-context'

/** The pending-row setters the tree provider owns; passed in so the import
 *  orchestration stays a plain, testable function. */
export interface PendingRowHooks {
  addPending: (tempId: string, entry: PendingCreate) => void
  setError: (tempId: string, error: string) => void
  clearPending: (tempId: string) => void
}

/**
 * Optimistically show a spinner row per imported branch (the same loading
 * affordance as a new-workspace create), then fire the batch import (202). Each
 * row is placed at the repo root; because the daemon decides each branch's
 * PR-parent server-side, the real WorkspaceDTO may land nested elsewhere, so a
 * row is matched on branch alone and dropped the instant its workspace appears
 * on the WS stream. A rejected request marks the rows errored AND toasts.
 *
 * The spinner rows depend on the daemon's contract that EVERY imported branch
 * produces a workspace — a branch it cannot materialise arrives as a placeholder
 * (empty localPath + heldByPath) rather than as nothing, which is what left
 * these rows spinning forever with nothing surfaced. Failures of the async
 * import therefore ride their own entity, not this catch.
 *
 * Returns the store unsubscribe for callers that need it (tests).
 */
export function performImportBranches(
  repoId: string,
  branches: string[],
  hooks: PendingRowHooks,
): () => void {
  const noop = () => {}
  if (branches.length === 0) return noop
  const projectId = projectIdForRepo(repoId)
  const rootParentId =
    useSidebarStore.getState().repos.find((r) => r.id === repoId)?.defaultWorkspaceId ?? ''

  const temps = branches.map((branch) => {
    const tempId = crypto.randomUUID()
    hooks.addPending(tempId, { repoId, parentId: rootParentId, branch })
    return { tempId, branch }
  })

  if (!projectId) {
    temps.forEach(({ tempId }) => hooks.setError(tempId, 'Unknown project'))
    return noop
  }

  const remaining = new Set(temps.map((t) => t.branch))
  const unsub = useSidebarStore.subscribe((state) => {
    const repo = state.repos.find((r) => r.id === repoId)
    if (!repo) return
    for (const { tempId, branch } of temps) {
      if (remaining.has(branch) && repo.workspaces.some((w) => w.branch === branch)) {
        hooks.clearPending(tempId)
        remaining.delete(branch)
      }
    }
    if (remaining.size === 0) unsub()
  })

  importBranches(projectId, repoId, branches).catch((err) => {
    unsub()
    const msg = err instanceof Error ? err.message : 'Import failed'
    temps.forEach(({ tempId }) => hooks.setError(tempId, msg))
    // The errored row only reads "failed", in a sidebar the user may not be
    // looking at. A refused import has to say why — the async failures ride
    // their workspace entity (placeholder + its own toast) instead.
    toast.error(`Couldn't import ${branches.length === 1 ? branches[0] : 'branches'}: ${msg}`)
  })

  return unsub
}

/**
 * Resolve the owning project id for a repo from the sidebar tree. Hierarchical
 * mutations need both ids; the tree always carries `projectId` from the §5
 * RepoDTO once the repo has seeded.
 */
export function projectIdForRepo(repoId: string): string | undefined {
  return useSidebarStore.getState().repos.find((r) => r.id === repoId)?.projectId
}

/**
 * Fire the hierarchical create mutation (202 Accepted, §3). No optimistic node
 * is added: the WorkspaceDTO arrives over the scoped WS stream (status 'new'
 * then the real status) and the WS-driven cache inserts it.
 */
export async function performCreateWorkspace(
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void> {
  const projectId = projectIdForRepo(repoId)
  if (!projectId) {
    console.error('Failed to create workspace: unknown project for repo', repoId)
    return
  }
  try {
    await postWorkspace(projectId, repoId, branch, { parentId })
  } catch (err) {
    console.error('Failed to create workspace:', err)
  }
}

/**
 * Fire the branch rename. The daemon renames the git branch AND relocates the
 * workspace's directory, then broadcasts the updated WorkspaceDTO — so, like
 * create and delete, there is NO optimistic write here. An optimistic relabel is
 * exactly what made rename look like it worked while changing nothing: the row
 * showed the new name until the next reseed put the old one back.
 *
 * A refusal (the branch is taken, the workspace is locked or is an adopted
 * checkout) comes back as a 409 whose message is written for the user, so it is
 * surfaced rather than logged — they just typed the name that was rejected.
 */
export async function performRenameWorkspaceBranch(wsId: string, branch: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  if (ws.branch === branch) return
  const projectId = repo.projectId
  if (!projectId) return
  try {
    await renameWorkspaceBranch(projectId, repo.id, wsId, branch)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to rename branch')
  }
}

/**
 * Fire a folder rename.
 *
 * A folder holds no branch and no worktree, so renaming one moves nothing on
 * disk: it is the same PATCH that files a folder somewhere else, carrying the
 * one field that changed. Like the branch rename there is no optimistic write —
 * the updated FolderDTO arrives on the stream, and relabelling the row here
 * would only mean showing a name the daemon may not have taken.
 */
export async function performRenameFolder(folderId: string, name: string): Promise<void> {
  const repo = useSidebarStore
    .getState()
    .repos.find((r) => r.folders?.some((f) => f.id === folderId))
  const folder = repo?.folders?.find((f) => f.id === folderId)
  if (!repo?.projectId || !folder) return
  if (folder.name === name) return
  try {
    await placeFolder(repo.projectId, repo.id, folderId, { name })
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to rename folder')
  }
}

/**
 * Rename whatever row `rowId` names.
 *
 * The sidebar has ONE rename gesture and one inline editor, so it needs one
 * place that knows a folder is not a branch. The id answers that on its own —
 * the two id spaces never overlap — which keeps the row itself from having to
 * carry a second, parallel rename path just to reach a different endpoint.
 */
export function performRenameRow(rowId: string, name: string): Promise<void> {
  const isFolder = useSidebarStore
    .getState()
    .repos.some((r) => r.folders?.some((f) => f.id === rowId))
  return isFolder ? performRenameFolder(rowId, name) : performRenameWorkspaceBranch(rowId, name)
}

/**
 * Fire the hierarchical delete mutation (202 Accepted, §3). Locked workspaces
 * are never deleted. No optimistic removal: the backend owns the cascade and
 * emits one status:'deleted' tombstone per removed id, which the WS-driven
 * cache applies. On failure the item stays in the list via WS non-arrival — no
 * toast needed.
 */
export async function performDeleteWorkspace(wsId: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  const projectId = repo.projectId
  if (!projectId) return
  try {
    await apiDeleteWorkspace(projectId, repo.id, wsId)
  } catch (err) {
    console.error('Failed to delete workspace:', err)
    // item stays in list via WS non-arrival — no toast needed
  }
}
