import { readSelect, selectRepos } from '@/core/entities/select'
import type { Repo } from '@/core/entities/repo-types'
import { applyFolderPlacements } from '@/lib/store/applied-placement'
import { getHomeTree, resolveHomeRowScope } from '@/core/entities/home-rows'
import { renameWorkspaceBranch, renameRepo, renameProject, setWorkspaceLock } from '@/lib/api'
import {
  createFolder,
  createHomeFolder,
  placeFolder,
  placeHomeFolder,
} from '@/lib/api/sidebar-placement'
import { renameChat } from '@/features/agent/api/agent-chat-commands'
import { toast } from '@/features/window/stores/toast-store'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { OwningChatNotRecordedError } from '@/lib/workspace-scope-url'
import { report } from '@/lib/errors/report'
import { hostIdForProject } from '@/lib/host-resolve'

// Every worktree verb is addressed through the workspace's owning chat, which the daemon mints
// on first read — so a row without one is still loading, never a dead end. Says so instead of
// surfacing the raw scope error.
export function chatNotLoadedYet(verb: string, branch: string | undefined): string {
  return `Can't ${verb} ${branch ?? 'this branch'} yet — its chat is still loading`
}

/** What a folder is called until the user says otherwise. */
const NEW_FOLDER_NAME = 'New folder'

/** A repo's project id off the sidebar tree; hierarchical mutations need both ids. */
export function projectIdForRepo(repoId: string): string | undefined {
  return readSelect(selectRepos).find((r) => r.id === repoId)?.projectId
}

// The daemon renames the git branch and broadcasts the updated workspace, so there is no
// optimistic write: a relabel would show a name that never landed. A 409 refusal (name taken,
// locked, adopted checkout) carries a user-facing message, so it is surfaced.
export async function performRenameWorkspaceBranch(wsId: string, branch: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  if (ws.branch === branch) return
  const projectId = repo.projectId
  if (!projectId) return
  try {
    await renameWorkspaceBranch(projectId, repo.id, wsId, branch)
  } catch (err) {
    if (err instanceof OwningChatNotRecordedError) {
      toast.error(chatNotLoadedYet('rename', ws.branch))
      return
    }
    report(err, { action: 'Failed to rename branch' })
  }
}

// A folder holds no worktree, so its rename is the same placement PATCH that refiles it, with only
// the name changed. It applies the daemon's committed `{folder, shifted}` at once.
export async function performRenameFolder(folderId: string, name: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.folders?.some((f) => f.id === folderId))
  const folder = repo?.folders?.find((f) => f.id === folderId)
  if (!repo?.projectId || !folder) return
  if (folder.name === name) return
  try {
    const {
      folder: updated,
      shifted,
      shiftedRows,
    } = await placeFolder(repo.projectId, repo.id, folderId, { name })
    await applyFolderPlacements(
      hostIdForProject(repo.projectId),
      [updated, ...shifted],
      shiftedRows,
    )
  } catch (err) {
    report(err, { action: 'Failed to rename folder' })
  }
}

// performRenameFolder for a project-home folder, which lives in no repo's `folders`.
export async function performRenameHomeFolder(
  projectId: string,
  folderId: string,
  name: string,
): Promise<void> {
  const folder = getHomeTree(projectId).folders.find((f) => f.id === folderId)
  if (!folder || folder.name === name) return
  try {
    const { folder: updated, shifted } = await placeHomeFolder(projectId, folderId, { name })
    await applyFolderPlacements(hostIdForProject(projectId), [updated, ...shifted])
  } catch (err) {
    report(err, { action: 'Failed to rename folder' })
  }
}

// Any workspace of `repo` whose scope is recorded — `.../chats` is repo-scoped, so which one
// builds the URL doesn't matter. The repo's own ids, never a chat's `workspaceId`: a chat may
// name a workspace in another repo (spec §9.2), which would address the wrong repo.
export function scopedWorkspaceIdOf(repo: Repo): string | undefined {
  return repo.defaultWorkspaceId ?? repo.workspaces[0]?.id
}

// A chat title lives on its own aggregate (no branch, no worktree), so this is the plain
// `POST .../chats/:id/rename`; the new title arrives on the chat's frame.
export async function performRenameChat(chatId: string, title: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.chats?.some((c) => c.id === chatId))
  const chat = repo?.chats?.find((c) => c.id === chatId)
  if (!repo || !chat) return
  // The rename dialog seeds from the row's label (`title || UNTITLED_CHAT_LABEL`), so compare
  // against the same fallback; otherwise a no-op Enter pins an untitled chat's title to
  // "Untitled chat" and the agent's auto-title can never replace it.
  if ((chat.title || UNTITLED_CHAT_LABEL) === title) return
  const wsId = scopedWorkspaceIdOf(repo)
  if (!wsId) return
  try {
    await renameChat(wsId, chatId, title)
  } catch (err) {
    report(err, { action: 'Failed to rename chat' })
  }
}

// performRenameChat for a project-home chat: no repo, so the home workspace names the route,
// and the chat's projects-scope frame settles the title.
export async function performRenameHomeChat(
  projectId: string,
  homeWorkspaceId: string,
  chatId: string,
  title: string,
): Promise<void> {
  const chat = getHomeTree(projectId).chats.find((c) => c.id === chatId)
  if (!chat) return
  if ((chat.title || UNTITLED_CHAT_LABEL) === title) return
  try {
    await renameChat(homeWorkspaceId, chatId, title)
  } catch (err) {
    report(err, { action: 'Failed to rename chat' })
  }
}

// Renames the repo's display name, never its branch: the project-home header row is the repo's
// default workspace, and renaming that row names the repo.
export async function performRenameRepo(repoId: string, name: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.id === repoId)
  if (!repo?.projectId) return
  if (repo.name === name) return
  try {
    await renameRepo(repo.projectId, repoId, name)
  } catch (err) {
    report(err, { action: 'Failed to rename repository' })
  }
}

// A project is not a SidebarRow (the project switcher renames it), so it bypasses performRenameRow.
export async function performRenameProject(projectId: string, name: string): Promise<void> {
  try {
    await renameProject(projectId, name)
  } catch (err) {
    report(err, { action: 'Failed to rename project' })
  }
}

// One rename gesture for every row; the id says which endpoint. Order matters: project-home rows
// (in no repo) first, then the repo header (its defaultWorkspaceId), then workspace rows — a
// locked branch renames its branch, any other carries its chat's title — then chats, folders.
export function performRenameRow(rowId: string, name: string): Promise<void> {
  const homeScope = resolveHomeRowScope(rowId)
  if (homeScope) {
    return homeScope.kind === 'chat'
      ? performRenameHomeChat(homeScope.projectId, homeScope.homeWorkspaceId, rowId, name)
      : performRenameHomeFolder(homeScope.projectId, rowId, name)
  }
  const state = { repos: readSelect(selectRepos) }
  // A branch row's id is its owning chat's, so translate before matching: otherwise the repo
  // header misses `defaultWorkspaceId`, falls into the chat case and retitles a conversation.
  const wsId = workspaceIdOfBranchRow(state.repos, rowId) ?? rowId
  const homeRepo = state.repos.find((r) => r.defaultWorkspaceId === wsId)
  if (homeRepo) return performRenameRepo(homeRepo.id, name)
  if (wsId !== rowId) {
    // A workspace-owning row: a locked branch keeps its branch-name label, so it renames the
    // branch; every other such row is labelled by its chat's title.
    const repo = state.repos.find((r) => r.workspaces.some((w) => w.id === wsId))
    const ws = repo?.workspaces.find((w) => w.id === wsId)
    if (ws?.status === 'locked') return performRenameWorkspaceBranch(wsId, name)
    return performRenameChat(rowId, name)
  }
  if (state.repos.some((r) => r.chats?.some((c) => c.id === rowId))) {
    return performRenameChat(rowId, name)
  }
  const isFolder = state.repos.some((r) => r.folders?.some((f) => f.id === rowId))
  return isFolder ? performRenameFolder(rowId, name) : performRenameWorkspaceBranch(wsId, name)
}

// Set or clear a workspace's lock; `locked: null` drops the user's override (see
// setWorkspaceLock). Takes a row id and translates it: a locked branch row is id'd by its owning
// chat, and without the translation a locked workspace could be neither unlocked nor deleted.
export async function performSetWorkspaceLock(
  rowId: string,
  locked: boolean | null,
): Promise<void> {
  const repos = readSelect(selectRepos)
  const wsId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  const repo = repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const projectId = repo?.projectId
  if (!repo || !projectId) return
  try {
    await setWorkspaceLock(wsId, locked)
  } catch (err) {
    if (err instanceof OwningChatNotRecordedError) {
      const branch = repo.workspaces.find((w) => w.id === wsId)?.branch
      toast.error(chatNotLoadedYet(locked === false ? 'unlock' : 'lock', branch))
      return
    }
    report(err, { action: 'Failed to update lock' })
  }
}

// Creates "New folder" under `rowId`; the repo header normalises to the repo root. The row is
// translated to a workspace only to find its repo — the `parentId` sent stays `rowId`, since
// `POST .../chats/folders` resolves it as a chat or folder and 404s on a workspace id.
export async function performCreateFolder(rowId: string): Promise<void> {
  const repos = readSelect(selectRepos)
  const wsId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  const repo = repos.find(
    (r) =>
      r.defaultWorkspaceId === wsId ||
      r.workspaces.some((w) => w.id === wsId) ||
      r.folders?.some((f) => f.id === rowId),
  )
  const projectId = repo?.projectId
  if (!repo || !projectId) return
  const folderParentId = wsId === repo.defaultWorkspaceId ? '' : rowId
  try {
    // Applied directly: no push channel exists for folders, so the response IS the confirmation.
    const { folder, shifted, shiftedRows } = await createFolder(
      projectId,
      repo.id,
      NEW_FOLDER_NAME,
      folderParentId,
    )
    await applyFolderPlacements(hostIdForProject(projectId), [folder, ...shifted], shiftedRows)
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}

// performCreateFolder for a plain chat row, so a project with no folders yet still offers one on
// right-click. A chat id never translates to a workspace, and a chat has no folder anchor of its
// own, so its folder always lands at the repo root.
export async function performCreateFolderFromChat(chatId: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.chats?.some((c) => c.id === chatId))
  const projectId = repo?.projectId
  if (!repo || !projectId) return
  try {
    const { folder, shifted, shiftedRows } = await createFolder(
      projectId,
      repo.id,
      NEW_FOLDER_NAME,
      '',
    )
    await applyFolderPlacements(hostIdForProject(projectId), [folder, ...shifted], shiftedRows)
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}

// performCreateFolder for the project-home workspace, which no repo tree contains: `parentId` is
// '' (the home root) or a home folder. The response IS the confirmation, so it is applied directly.
export async function performCreateHomeFolder(projectId: string, parentId = ''): Promise<void> {
  try {
    const { folder, shifted } = await createHomeFolder(projectId, NEW_FOLDER_NAME, parentId)
    await applyFolderPlacements(hostIdForProject(projectId), [folder, ...shifted])
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}
