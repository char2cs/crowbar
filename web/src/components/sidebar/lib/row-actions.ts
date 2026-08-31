import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { renameWorkspaceBranch, renameRepo, setWorkspaceLock, importBranches } from '@/lib/api'
import { createFolder, placeFolder } from '@/lib/api/sidebar-placement'
import { renameChat } from '@/features/agent/api/agent-api'
import { toast } from '@/features/window/stores/toast-store'

/** What a folder is called until the user says otherwise (matches the
 *  deleted workspace-tree-context.tsx's NEW_FOLDER_NAME). */
const NEW_FOLDER_NAME = 'New folder'

/**
 * Resolve the owning project id for a repo from the sidebar tree. Hierarchical
 * mutations need both ids; the tree always carries `projectId` from the §5
 * RepoDTO once the repo has seeded.
 */
export function projectIdForRepo(repoId: string): string | undefined {
  return useSidebarStore.getState().repos.find((r) => r.id === repoId)?.projectId
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
 * one field that changed.
 *
 * Unlike the branch rename, this DOES apply its own response: folders lost
 * their dedicated push channel (Task 34 — the backend plan that carried it is
 * closed), so the PATCH's `{folder, shifted}` answer is the only confirmation
 * this edit ever gets. Applying it is not optimistic — it is the daemon's own
 * already-committed state, arriving over the request instead of a stream.
 *
 * The direct `applyFolderDTO` write is instant visual feedback only — it
 * touches `useSidebarStore`, never the `crowbar_folders` IndexedDB cache that
 * every tree REBUILD reads from exclusively (`readVisibleRepoTree`). Without
 * the `bump` below, the very next rebuild — which fires for reasons that have
 * nothing to do with this edit, e.g. any repo's `defaultWorking` flipping —
 * would silently revert it, because the cache was never told. `bump` routes
 * through the same reseed mechanism `app-sync-provider.tsx` already built for
 * "another window's change eventually catches up," which writes the cache
 * authoritatively; the acting window now uses it too, immediately.
 */
export async function performRenameFolder(folderId: string, name: string): Promise<void> {
  const repo = useSidebarStore
    .getState()
    .repos.find((r) => r.folders?.some((f) => f.id === folderId))
  const folder = repo?.folders?.find((f) => f.id === folderId)
  if (!repo?.projectId || !folder) return
  if (folder.name === name) return
  try {
    const { folder: updated, shifted } = await placeFolder(repo.projectId, repo.id, folderId, {
      name,
    })
    const apply = useSidebarStore.getState().applyFolderDTO
    apply(updated)
    shifted.forEach(apply)
    useFolderSignalStore.getState().bump(repo.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to rename folder')
  }
}

/**
 * Any workspace of `repo` whose scope is recorded — all `chatBase` needs.
 *
 * `.../chats` is REPO-scoped (Task 17), so which of a repo's workspaces the
 * URL is built from cannot change the endpoint reached; `chatBase` only needs
 * one whose project/repo scope was recorded, and `recordRepoScopes` records
 * every one of these on each seed.
 *
 * The repo's OWN ids, never the chat's `workspaceId`: a chat may legitimately
 * name a workspace in another repo (spec §9.2's open set), and building this
 * repo's URL from that id would either 404 or address the wrong repo.
 */
function scopedWorkspaceIdOf(repo: Repo): string | undefined {
  return repo.defaultWorkspaceId ?? repo.workspaces[0]?.id
}

/**
 * Fire a chat rename.
 *
 * A chat's title is a field on its own aggregate — no branch, no directory, no
 * git — so this is the plain `POST .../chats/:id/rename`, not the branch
 * rename's move-the-worktree-on-disk operation.
 *
 * Bumping the tree signal afterwards is the same reasoning
 * `performRenameFolder` above records: the sidebar's copy of a chat row is
 * rebuilt from the `crowbar_chats` cache, and only a reseed writes that. The
 * daemon does broadcast `title_set`, which bumps the same signal — but only
 * onto clients with a workspace of this repo mounted, and the acting user
 * should not wait on a frame to see the name they just typed. A reseed that
 * lands before the projection catches up is harmless: the frame bumps again.
 */
export async function performRenameChat(chatId: string, title: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.chats?.some((c) => c.id === chatId))
  const chat = repo?.chats?.find((c) => c.id === chatId)
  if (!repo || !chat) return
  if (chat.title === title) return
  const wsId = scopedWorkspaceIdOf(repo)
  if (!wsId) return
  try {
    await renameChat(wsId, chatId, title)
    useFolderSignalStore.getState().bump(repo.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to rename chat')
  }
}

/**
 * Fire a repo rename — the repo's own display name, not its checked-out
 * branch. The project-home row IS the repo's default workspace (its own
 * checkout); renaming that row names the repo, exactly as the deleted
 * `repo-section.tsx`'s header row did ("Repo rename stays on the [repo
 * name], not the branch") — it never called the branch-rename endpoint.
 */
export async function performRenameRepo(repoId: string, name: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.id === repoId)
  if (!repo?.projectId) return
  if (repo.name === name) return
  try {
    await renameRepo(repo.projectId, repoId, name)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to rename repository')
  }
}

/**
 * Rename whatever row `rowId` names.
 *
 * The sidebar has ONE rename gesture and one inline editor, so it needs one
 * place that knows a folder is not a branch. The id answers that on its own —
 * the two id spaces never overlap — which keeps the row itself from having to
 * carry a second, parallel rename path just to reach a different endpoint.
 *
 * A third id space joins those two here: the project-home row's id is a
 * repo's `defaultWorkspaceId`, never a member of that repo's `workspaces`
 * array (it's the header, not a tree row) — so it has to be checked before
 * falling through to the branch-rename path, which would otherwise silently
 * find no matching workspace and do nothing.
 *
 * A FOURTH id space is a chat, and it has to be checked for exactly the reason
 * the third does — with a sharper failure. Renaming a chat row fell through to
 * `performRenameWorkspaceBranch`, which found no workspace by that id and
 * returned: no request, no error, and the name the user had just typed into
 * the inline editor simply gone. A rename that silently discards what was
 * typed is indistinguishable from one that worked and was then reverted.
 */
export function performRenameRow(rowId: string, name: string): Promise<void> {
  const state = useSidebarStore.getState()
  const homeRepo = state.repos.find((r) => r.defaultWorkspaceId === rowId)
  if (homeRepo) return performRenameRepo(homeRepo.id, name)
  if (state.repos.some((r) => r.chats?.some((c) => c.id === rowId))) {
    return performRenameChat(rowId, name)
  }
  const isFolder = state.repos.some((r) => r.folders?.some((f) => f.id === rowId))
  return isFolder ? performRenameFolder(rowId, name) : performRenameWorkspaceBranch(rowId, name)
}

/**
 * Set (or clear) a workspace's lock from the row context menu. `locked: null`
 * drops the user's override rather than forcing false — see `setWorkspaceLock`'s
 * own doc for why that third state exists.
 */
export async function performSetWorkspaceLock(wsId: string, locked: boolean | null): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const projectId = repo?.projectId
  if (!repo || !projectId) return
  try {
    await setWorkspaceLock(projectId, repo.id, wsId, locked)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to update lock')
  }
}

/**
 * Fire the batch branch import (202 Accepted). No optimistic spinner rows —
 * unlike the deleted PendingRowHooks version, this relies on the same
 * WS-driven cache that already surfaces create/rename/delete with no
 * optimistic write of their own.
 */
export async function performImportBranches(repoId: string, branches: string[]): Promise<void> {
  if (branches.length === 0) return
  const projectId = projectIdForRepo(repoId)
  if (!projectId) return
  try {
    await importBranches(projectId, repoId, branches)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to import branches')
  }
}

/**
 * Create a folder named 'New folder' under `parentId` — how `createFolder`
 * (`@/lib/api/sidebar-placement`) gets a live caller again now that the
 * deleted drag-driven "group into folder" gesture is Part G's to rebuild.
 *
 * `parentId` is root-normalised exactly as the deleted confirmCreate's folder
 * branch did (`folderParentFor`): a repo's default (home) workspace is the
 * header row, not a real placement target for the daemon, so starting a
 * folder there lands it at the repo root instead of naming a parent that
 * doesn't exist in that space.
 */
export async function performCreateFolder(parentId: string): Promise<void> {
  const repo = useSidebarStore
    .getState()
    .repos.find(
      (r) =>
        r.defaultWorkspaceId === parentId ||
        r.workspaces.some((w) => w.id === parentId) ||
        r.folders?.some((f) => f.id === parentId),
    )
  const projectId = repo?.projectId
  if (!repo || !projectId) return
  const folderParentId = parentId === repo.defaultWorkspaceId ? '' : parentId
  try {
    // Applied directly for the same reason performRenameFolder does: no
    // dedicated push channel exists for folders any more, so the response IS
    // the confirmation. `bump` (see performRenameFolder's doc) writes
    // `crowbar_folders` too — without it, the new row survives only until the
    // next unrelated tree rebuild silently drops it again.
    const { folder, shifted } = await createFolder(
      projectId,
      repo.id,
      NEW_FOLDER_NAME,
      folderParentId,
    )
    const apply = useSidebarStore.getState().applyFolderDTO
    apply(folder)
    shifted.forEach(apply)
    useFolderSignalStore.getState().bump(repo.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to create folder')
  }
}
