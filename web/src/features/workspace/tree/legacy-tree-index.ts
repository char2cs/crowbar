import type { Chat, Folder, Repo, Workspace } from '@/core/entities/repo-types'
import { createdInstant } from '@/lib/store/created-instant'
import { resolveHomeOwnerId, resolveOwnerChats } from '@/components/sidebar/lib/workspace-owners'
import { indexNodes, type TreeIndex, type TreeNode } from './tree-index'

// Today's placement fields (folderId, fork parent, chat parent, workspace ground) turned
// into explicit containers once, so row derivation never reads a fallback. Goes when the
// daemon's Tree reaches the client.

/** A project's home chats and folders, and the workspace and owner they hang off. */
export interface LegacyHome {
  workspaceId: string
  /** The owner GET /home named; the chat list's own marker is the fallback. */
  owningChatId?: string
  chats: readonly Chat[]
  folders: readonly Folder[]
}

type Built =
  | { kind: 'workspace'; id: string; workspace: Workspace; children: Built[] }
  | { kind: 'folder'; id: string; folder: Folder; children: Built[] }
  | { kind: 'chat'; id: string; chat: Chat; children: Built[] }

const NO_ORDER = Number.MAX_SAFE_INTEGER
const KIND_RANK = { folder: 0, workspace: 1, chat: 2 } as const

function orderOf(node: Built): number | undefined {
  if (node.kind === 'folder') return node.folder.order
  return node.kind === 'chat' ? node.chat.order : node.workspace.order
}

function createdOf(node: Built): string | undefined {
  if (node.kind === 'folder') return node.folder.createdAt
  return node.kind === 'chat' ? node.chat.createdAt : node.workspace.createdAt
}

/** The workspace whose sibling space holds `id`, walking up through folders and chats. */
function anchorOf(
  id: string,
  nodes: ReadonlyMap<string, Built>,
  parentOf: ReadonlyMap<string, string | undefined>,
): string | undefined {
  const visited = new Set<string>()
  let cursor: string | undefined = id
  while (cursor && !visited.has(cursor)) {
    visited.add(cursor)
    const node = nodes.get(cursor)
    if (!node) return undefined
    if (node.kind === 'workspace') return node.id
    cursor = parentOf.get(cursor)
  }
  return undefined
}

/** Where a workspace hangs: its folder, unless that folder would split its fork chain. */
function workspaceParent(
  ws: Workspace,
  nodes: ReadonlyMap<string, Built>,
  parentOf: ReadonlyMap<string, string | undefined>,
): string | undefined {
  const forkParent = ws.parentId && nodes.has(ws.parentId) ? ws.parentId : undefined
  const folder = ws.folderId && nodes.has(ws.folderId) ? ws.folderId : undefined
  // A fork root has no chain to split, so its folder wins wherever the folder sits.
  if (folder && (forkParent === undefined || anchorOf(folder, nodes, parentOf) === forkParent))
    return folder
  return forkParent
}

function closesCycle(id: string, parentOf: ReadonlyMap<string, string | undefined>): boolean {
  const visited = new Set<string>([id])
  let cursor = parentOf.get(id)
  while (cursor) {
    if (visited.has(cursor)) return true
    visited.add(cursor)
    cursor = parentOf.get(cursor)
  }
  return false
}

function sortLevel(level: Built[], arrival: ReadonlyMap<string, number>): void {
  level.sort((a, b) => {
    const byOrder = (orderOf(a) ?? NO_ORDER) - (orderOf(b) ?? NO_ORDER)
    if (byOrder !== 0) return byOrder
    const byKind = KIND_RANK[a.kind] - KIND_RANK[b.kind]
    if (byKind !== 0) return byKind
    const byCreated = createdInstant(createdOf(a)) - createdInstant(createdOf(b))
    if (byCreated !== 0) return byCreated
    return (arrival.get(a.id) ?? 0) - (arrival.get(b.id) ?? 0)
  })
  for (const node of level) sortLevel(node.children, arrival)
}

/** One scope's rows as a forest: a parent that names nothing here, or closes a cycle, roots. */
function buildForest(
  workspaces: readonly Workspace[],
  folders: readonly Folder[],
  chats: readonly Chat[],
): Built[] {
  const nodes = new Map<string, Built>()
  const arrival = new Map<string, number>()
  const parentOf = new Map<string, string | undefined>()
  // Folders, then branches, then chats: the arrival tiebreak of an undragged level.
  for (const folder of folders)
    nodes.set(folder.id, { kind: 'folder', id: folder.id, folder, children: [] })
  for (const ws of workspaces)
    nodes.set(ws.id, { kind: 'workspace', id: ws.id, workspace: ws, children: [] })
  for (const chat of chats) nodes.set(chat.id, { kind: 'chat', id: chat.id, chat, children: [] })
  for (const id of nodes.keys()) arrival.set(id, arrival.size)

  for (const folder of folders) parentOf.set(folder.id, folder.parentId || undefined)
  for (const chat of chats) {
    const filed = chat.parentId && nodes.has(chat.parentId) ? chat.parentId : undefined
    const ground = chat.workspaceId && nodes.has(chat.workspaceId) ? chat.workspaceId : undefined
    parentOf.set(chat.id, filed ?? ground)
  }
  for (const ws of workspaces) parentOf.set(ws.id, workspaceParent(ws, nodes, parentOf))

  const roots: Built[] = []
  for (const [id, node] of nodes) {
    const parentId = parentOf.get(id)
    const parent = parentId ? nodes.get(parentId) : undefined
    if (!parent || parent === node || closesCycle(id, parentOf)) roots.push(node)
    else parent.children.push(node)
  }
  sortLevel(roots, arrival)
  return roots
}

/** A workspace and the chat that owns it are one node: the owner's threads join the workspace's. */
function foldOwners(roots: Built[], ownerChats: ReadonlyMap<string, string>): Built[] {
  const owners = new Set(ownerChats.values())
  const detached = new Map<string, Built>()
  const strip = (level: Built[]): Built[] => {
    const kept: Built[] = []
    for (const node of level) {
      const children = strip(node.children)
      if (node.kind === 'chat' && owners.has(node.id)) detached.set(node.id, { ...node, children })
      else kept.push({ ...node, children })
    }
    return kept
  }
  const merge = (level: Built[]): Built[] =>
    level.map((node) => {
      const children = merge(node.children)
      const ownerId = node.kind === 'workspace' ? ownerChats.get(node.id) : undefined
      const owner = ownerId ? detached.get(ownerId) : undefined
      return { ...node, children: owner ? [...children, ...merge(owner.children)] : children }
    })
  return merge(strip(roots))
}

function emit(out: TreeNode[], level: readonly Built[], parentId: string): void {
  level.forEach((node, index) => {
    out.push({
      id: node.id,
      kind: node.kind,
      parentId,
      order: orderOf(node) ?? index,
      createdAt: createdOf(node),
      ...(node.kind === 'folder' ? { name: node.folder.name } : {}),
    })
    emit(out, node.children, node.id)
  })
}

/**
 * The one home chat kept off the tree: the home worktree's owner. A chat GET /home names
 * that the list marks as NOT owning the worktree is a conversation, and is drawn.
 */
export function homeOwnerRowId(
  homeWorkspaceId: string,
  chats: readonly Chat[],
  owningChatId?: string,
): string {
  const named = owningChatId ? chats.find((c) => c.id === owningChatId) : undefined
  if (named?.ownsWorktree === false) return resolveHomeOwnerId(homeWorkspaceId, undefined, chats)
  return resolveHomeOwnerId(homeWorkspaceId, owningChatId, chats)
}

/** A repo's own members: never another repo's chats, never its home or a tombstone. */
export function repoMembers(repo: Repo): { chats: Chat[]; workspaces: Workspace[] } {
  const homeId = repo.defaultWorkspaceId
  return {
    chats: (repo.chats ?? []).filter((c) => c.repoId === repo.id),
    workspaces: repo.workspaces.filter((w) => w.status !== 'deleted' && w.id !== homeId),
  }
}

/** One project's tree from today's stores: home rows at the root beside its repos. */
export function legacyTreeIndex(repos: readonly Repo[], home: LegacyHome | null): TreeIndex {
  const out: TreeNode[] = []
  if (home) {
    const owner = homeOwnerRowId(home.workspaceId, home.chats, home.owningChatId)
    emit(
      out,
      buildForest(
        [],
        home.folders,
        home.chats.filter((c) => c.id !== owner),
      ),
      '',
    )
  }
  const homeFolders = new Set(home?.folders.map((f) => f.id))
  for (const repo of repos) {
    const { chats, workspaces } = repoMembers(repo)
    const homeId = repo.defaultWorkspaceId
    const owner = homeId ? resolveHomeOwnerId(homeId, repo.defaultOwningChatId, chats) : null
    out.push({
      id: repo.id,
      kind: 'repo',
      parentId: repo.folderId && homeFolders.has(repo.folderId) ? repo.folderId : '',
      order: repo.order ?? 0,
    })
    const forest = buildForest(
      workspaces,
      repo.folders ?? [],
      chats.filter((c) => c.id !== owner),
    )
    emit(out, foldOwners(forest, resolveOwnerChats(workspaces, chats)), repo.id)
  }
  return indexNodes(out)
}
