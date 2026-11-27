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
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { hostIdForProject } from '@/lib/host-resolve'

export function chatNotLoadedYet(verb: string, branch: string | undefined): string {
  return `Can't ${verb} ${branch ?? 'this branch'} yet — its chat is still loading`
}

const NEW_FOLDER_NAME = 'New folder'

function nameNewFolder(folderId: string): void {
  treeUiStore.getState().startRenaming(folderId, 'tree')
}

async function performRenameWorkspaceBranch(wsId: string, branch: string): Promise<void> {
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

async function performRenameFolder(folderId: string, name: string): Promise<void> {
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

async function performRenameHomeFolder(
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

function scopedWorkspaceIdOf(repo: Repo): string | undefined {
  return repo.defaultWorkspaceId ?? repo.workspaces[0]?.id
}

export async function performRenameChat(chatId: string, title: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.chats?.some((c) => c.id === chatId))
  const chat = repo?.chats?.find((c) => c.id === chatId)
  if (!repo || !chat) return
  if ((chat.title || UNTITLED_CHAT_LABEL) === title) return
  const wsId = scopedWorkspaceIdOf(repo)
  if (!wsId) return
  try {
    await renameChat(wsId, chatId, title)
  } catch (err) {
    report(err, { action: 'Failed to rename chat' })
  }
}

async function performRenameHomeChat(
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

async function performRenameRepo(repoId: string, name: string): Promise<void> {
  const repo = readSelect(selectRepos).find((r) => r.id === repoId)
  if (!repo?.projectId) return
  if (repo.name === name) return
  try {
    await renameRepo(repo.projectId, repoId, name)
  } catch (err) {
    report(err, { action: 'Failed to rename repository' })
  }
}

export async function performRenameProject(projectId: string, name: string): Promise<void> {
  try {
    await renameProject(projectId, name)
  } catch (err) {
    report(err, { action: 'Failed to rename project' })
  }
}

export function performRenameRow(rowId: string, name: string): Promise<void> {
  const homeScope = resolveHomeRowScope(rowId)
  if (homeScope) {
    return homeScope.kind === 'chat'
      ? performRenameHomeChat(homeScope.projectId, homeScope.homeWorkspaceId, rowId, name)
      : performRenameHomeFolder(homeScope.projectId, rowId, name)
  }
  const state = { repos: readSelect(selectRepos) }
  const wsId = workspaceIdOfBranchRow(state.repos, rowId) ?? rowId
  const homeRepo = state.repos.find((r) => r.defaultWorkspaceId === wsId)
  if (homeRepo) return performRenameRepo(homeRepo.id, name)
  if (wsId !== rowId) {
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
    const { folder, shifted, shiftedRows } = await createFolder(
      projectId,
      repo.id,
      NEW_FOLDER_NAME,
      folderParentId,
    )
    await applyFolderPlacements(hostIdForProject(projectId), [folder, ...shifted], shiftedRows)
    nameNewFolder(folder.id)
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}

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
    nameNewFolder(folder.id)
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}

export async function performCreateHomeFolder(projectId: string, parentId = ''): Promise<void> {
  try {
    const { folder, shifted } = await createHomeFolder(projectId, NEW_FOLDER_NAME, parentId)
    await applyFolderPlacements(hostIdForProject(projectId), [folder, ...shifted])
    nameNewFolder(folder.id)
  } catch (err) {
    report(err, { action: 'Failed to create folder' })
  }
}
