import type { EntityState } from '@/core/entities/entity-store'
import type { ChatEntity } from '@/core/entities/chat-entity'
import type { Chat, Folder, Repo } from '@/core/entities/repo-types'
import { mapChat, type AgentChat } from '@/features/agent/api/agent-chat-reads'
import {
  buildRepoTree,
  sortByPlacement,
  toSidebarChat,
  toSidebarFolder,
} from '@/lib/store/build-repo-tree'
import type { Project, WorkspaceDTO } from '@/lib/types'
import { LOCAL_HOST } from '@/features/hosts/types'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

/** A project's home chats and folders. */
export interface HomeTree {
  chats: Chat[]
  folders: Folder[]
}

export const EMPTY_CHATS: Chat[] = []
export const EMPTY_FOLDERS: Folder[] = []
export const EMPTY_HOME_TREE: HomeTree = { chats: EMPTY_CHATS, folders: EMPTY_FOLDERS }

/** The sidebar as the entity store holds it, in the shapes its readers draw. */
export interface SidebarViews {
  projects: Project[]
  repos: Repo[]
  homeTrees: Record<string, HomeTree>
  /** Every chat as the chat surfaces read it, by id. */
  chats: Record<string, AgentChat>
  /** runner id → the chat it runs. */
  byRunner: Record<string, string>
  /** The projects scope's snapshot has landed. */
  synced: boolean
}

type ByKind = EntityState['byKind']

function hostRank(hostId: string): number {
  if (hostId === LOCAL_HOST.id) return -1
  const index = useHostConfigStore.getState().hosts.findIndex((host) => host.id === hostId)
  return index === -1 ? Number.MAX_SAFE_INTEGER : index
}

/** Grouped by host (each daemon numbers only its own projects), then as that daemon orders them. */
function byOrder(a: Project, b: Project): number {
  const h = hostRank(a.hostId) - hostRank(b.hostId)
  if (h !== 0) return h
  const d = (a.order ?? 0) - (b.order ?? 0)
  if (d !== 0) return d
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

function sidebarChat(chat: ChatEntity, owners: ReadonlyMap<string, string>): Chat {
  return toSidebarChat({
    id: chat.id,
    repoId: chat.repoId,
    projectId: chat.projectId,
    type: chat.type,
    workspaceId: chat.workspaceId ?? '',
    ownsWorktree: !!chat.workspaceId && owners.get(chat.workspaceId) === chat.id,
    parentId: chat.parentId ?? '',
    title: chat.title,
    order: chat.order ?? 0,
    createdAt: chat.createdAt,
  })
}

// One AgentChat per chat record, so a chat whose row did not change keeps its object.
const agentChats = new WeakMap<object, AgentChat>()

function agentChatOf(record: { data: ChatEntity }): AgentChat {
  let chat = agentChats.get(record)
  if (!chat) agentChats.set(record, (chat = mapChat(record.data)))
  return chat
}

function isSameRow<T extends object>(existing: T, incoming: T): boolean {
  const keys = Object.keys(incoming) as Array<keyof T>
  return (
    keys.length === Object.keys(existing).length && keys.every((k) => existing[k] === incoming[k])
  )
}

/** `incoming`, keeping each row of `existing` it matches field for field, and `existing`
 *  itself when nothing changed: a frame for one row re-renders that row alone. */
function reconcileRows<T extends { id: string }>(existing: readonly T[], incoming: T[]): T[] {
  const byId = new Map(existing.map((row) => [row.id, row]))
  const next = incoming.map((row) => {
    const current = byId.get(row.id)
    return current && isSameRow(current, row) ? current : row
  })
  return next.length === existing.length && next.every((row, i) => row === existing[i])
    ? (existing as T[])
    : next
}

const NESTED = new Set(['workspaces', 'folders', 'chats'])

function reconcileRepo(current: Repo | undefined, repo: Repo): Repo {
  if (!current) return repo
  const workspaces = reconcileRows(current.workspaces, repo.workspaces)
  const folders = repo.folders && reconcileRows(current.folders ?? EMPTY_FOLDERS, repo.folders)
  const chats = repo.chats && reconcileRows(current.chats ?? EMPTY_CHATS, repo.chats)
  const keys = new Set([...Object.keys(current), ...Object.keys(repo)])
  const same = [...keys].every(
    (k) => NESTED.has(k) || current[k as keyof Repo] === repo[k as keyof Repo],
  )
  if (same && workspaces === current.workspaces && folders === current.folders) {
    if (chats === current.chats) return current
  }
  return {
    ...repo,
    workspaces,
    ...(folders ? { folders } : {}),
    ...(chats ? { chats } : {}),
  }
}

function reconcile(previous: SidebarViews | undefined, next: SidebarViews): SidebarViews {
  if (!previous) return next
  const repoById = new Map(previous.repos.map((r) => [r.id, r]))
  const homeTrees: Record<string, HomeTree> = {}
  for (const [id, tree] of Object.entries(next.homeTrees)) {
    const held = previous.homeTrees[id] ?? EMPTY_HOME_TREE
    const chats = reconcileRows(held.chats, tree.chats)
    const folders = reconcileRows(held.folders, tree.folders)
    homeTrees[id] = chats === held.chats && folders === held.folders ? held : { chats, folders }
  }
  return {
    ...next,
    projects: reconcileRows(previous.projects, next.projects),
    repos: reconcileRows(
      previous.repos,
      next.repos.map((r) => reconcileRepo(repoById.get(r.id), r)),
    ),
    homeTrees,
  }
}

function derive(k: ByKind, synced: boolean): SidebarViews {
  const workspaces: WorkspaceDTO[] = Object.values(k.workspace).map((r) => r.data)
  const owners = new Map(workspaces.map((w) => [w.id, w.owningChatId ?? '']))
  const homeTrees: Record<string, HomeTree> = {}
  const home = (projectId: string): HomeTree =>
    (homeTrees[projectId] ??= { chats: [], folders: [] })
  const repoFolders: Folder[] = []
  for (const data of sortByPlacement(Object.values(k.folder).map((r) => r.data))) {
    if (data.repoId) repoFolders.push(toSidebarFolder(data))
    else home(data.projectId).folders.push(toSidebarFolder(data))
  }
  const repoChats: Chat[] = []
  const chats: Record<string, AgentChat> = {}
  const byRunner: Record<string, string> = {}
  for (const record of Object.values(k.chat)) {
    const chat = agentChatOf(record)
    chats[chat.id] = chat
    if (chat.liveRunnerId) byRunner[chat.liveRunnerId] = chat.id
  }
  for (const data of sortByPlacement(Object.values(k.chat).map((r) => r.data))) {
    const row = sidebarChat(data, owners)
    if (data.repoId) repoChats.push(row)
    else home(data.projectId).chats.push(row)
  }
  // A twin (one id on two hosts, as a copied database makes) is drawn once, at its first host.
  const seen = new Set<string>()
  const projects = Object.values(k.project)
    .map((r) => r.data)
    .sort(byOrder)
    .filter((p) => !seen.has(p.id) && !!seen.add(p.id))
  for (const p of projects) home(p.id)
  return {
    projects,
    repos: buildRepoTree(
      Object.values(k.repo).map((r) => r.data),
      workspaces,
      repoFolders,
      repoChats,
    ),
    homeTrees,
    chats,
    byRunner,
    synced,
  }
}

let memo: { key: unknown[]; views: SidebarViews } | undefined

/** The sidebar's views of `s`, the same object until a kind they read changes. */
export function viewsFromEntities(s: EntityState): SidebarViews {
  const k = s.byKind
  const hosts = useHostConfigStore.getState().hosts
  const key = [k.project, k.repo, k.workspace, k.chat, k.folder, !!s.loaded.projects, hosts]
  if (memo && memo.key.length === key.length && memo.key.every((part, i) => part === key[i])) {
    return memo.views
  }
  memo = { key, views: reconcile(memo?.views, derive(k, !!s.loaded.projects)) }
  return memo.views
}
