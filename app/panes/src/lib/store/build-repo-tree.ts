import { isHostResolved } from '@/lib/host-resolve'
import { projectAssetURL } from '@/lib/project-url'
import { createdInstant } from '@/lib/store/created-instant'
import type { Chat, Folder, Repo, Workspace, WorkspaceStatus } from '@/core/entities/repo-types'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

export type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

const AVATAR_COLORS = [
  'bg-indigo-700',
  'bg-emerald-700',
  'bg-orange-700',
  'bg-sky-700',
  'bg-rose-700',
  'bg-violet-700',
  'bg-teal-700',
  'bg-amber-700',
]

function repoAvatarLabel(name: string): string {
  const words = name
    .replace(/[^a-zA-Z\s]/g, ' ')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (words.length === 0) return 'R'
  if (words.length === 1) return words[0][0].toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function repoAvatarColor(name: string): string {
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) & 0xffffff
  return AVATAR_COLORS[hash % AVATAR_COLORS.length]
}

function toSidebarStatus(ws: WorkspaceDTO): WorkspaceStatus {
  return ws.status as WorkspaceStatus
}

function repoAvatarURL(repo: RepoDTO): string | undefined {
  if (repo.avatarEmoji) return `emoji:${repo.avatarEmoji}`
  if (!repo.avatarUrl || !isHostResolved({ projectId: repo.projectId })) return undefined
  return projectAssetURL(repo.projectId, repo.avatarUrl)
}

function toSidebarWorkspace(ws: WorkspaceDTO): Workspace {
  return {
    id: ws.id,
    branch: ws.branch,
    parentId: ws.parentId || undefined,
    folderId: ws.folderId ?? '',
    order: ws.order ?? 0,
    status: toSidebarStatus(ws),
    added: ws.added,
    deleted: ws.deleted,
    working: ws.working,
    canMergeLocally: ws.canMergeLocally,
    mergeConflicts: ws.mergeConflicts,
    parentBranch: ws.parentBranch || undefined,
    prUrl: ws.prUrl || undefined,
    heldByPath: ws.heldByPath ?? '',
    provisioning: ws.provisioning,
    age: '',
    localPath: ws.localPath || undefined,
    owningChatId: ws.owningChatId ?? '',
    createdAt: ws.createdAt,
  }
}

export function toSidebarFolder(folder: FolderDTO): Folder {
  return {
    id: folder.id,
    repoId: folder.repoId,
    parentId: folder.parentId || undefined,
    name: folder.name,
    order: folder.order ?? 0,
    createdAt: folder.createdAt,
  }
}

export function toSidebarChat(chat: ChatDTO): Chat {
  return {
    id: chat.id,
    repoId: chat.repoId,
    type: chat.type,
    parentId: chat.parentId || undefined,
    workspaceId: chat.workspaceId || undefined,
    ...(chat.ownsWorktree === undefined ? {} : { ownsWorktree: chat.ownsWorktree }),
    title: chat.title,
    order: chat.order ?? 0,
    createdAt: chat.createdAt,
  }
}

const NO_ORDER = Number.MAX_SAFE_INTEGER

function compareByPlacement(
  a: { id: string; order?: number; createdAt?: string },
  b: { id: string; order?: number; createdAt?: string },
): number {
  const byOrder = (a.order ?? NO_ORDER) - (b.order ?? NO_ORDER)
  if (byOrder !== 0) return byOrder
  const byCreated = createdInstant(a.createdAt) - createdInstant(b.createdAt)
  if (byCreated !== 0) return byCreated
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

export function sortByPlacement<T extends { id: string; order?: number; createdAt?: string }>(
  rows: readonly T[],
): T[] {
  return [...rows].sort(compareByPlacement)
}

function toSidebarRepo(
  repo: RepoDTO,
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo {
  const repoWs = sortByPlacement(workspaces.filter((ws) => ws.repoId === repo.id))
  const repoFolders = folders.filter((folder) => folder.repoId === repo.id)
  const repoChats = chats.filter((chat) => chat.repoId === repo.id)
  const defaultWs = repoWs.find((ws) => ws.isDefault)
  const sidebarWorkspaces: Workspace[] = []
  for (const ws of repoWs) {
    if (!ws.isDefault) sidebarWorkspaces.push(toSidebarWorkspace(ws))
  }
  return {
    id: repo.id,
    projectId: repo.projectId,
    order: repo.order ?? 0,
    folderId: repo.folderId ?? '',
    name: repo.name,
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    avatarURL: repoAvatarURL(repo),
    workspaces: sidebarWorkspaces,
    ...(repoFolders.length > 0 ? { folders: repoFolders } : {}),
    ...(repoChats.length > 0 ? { chats: repoChats } : {}),
    ...(repo.deleting ? { deleting: true } : {}),
    ...(defaultWs
      ? {
          defaultWorkspaceId: defaultWs.id,
          defaultBranch: defaultWs.branch,
          defaultWorking: defaultWs.working,
          defaultWorkspaceStatus: toSidebarStatus(defaultWs),
          defaultOwningChatId: defaultWs.owningChatId ?? '',
        }
      : {}),
    ...(repo.path ? { localPath: repo.path } : {}),
  }
}

function sortReposByOrder(repos: readonly Repo[]): Repo[] {
  return sortByPlacement(repos)
}

export function buildRepoTree(
  repos: RepoDTO[],
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo[] {
  return sortReposByOrder(repos.map((repo) => toSidebarRepo(repo, workspaces, folders, chats)))
}
