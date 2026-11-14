import { isHostResolved } from '@/lib/host-resolve'
import { projectAssetURL } from '@/lib/project-url'
import { createdInstant } from '@/lib/store/created-instant'
import type { Chat, Folder, Repo, Workspace, WorkspaceStatus } from '@/core/entities/repo-types'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

// Re-export the canonical §5 DTOs so importers keep one source of truth.
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

// Status passes straight through (locked, pr-conflicts and deleted are WorkspaceStatus values);
// `working` is a separate in-progress flag, not a status.
function toSidebarStatus(ws: WorkspaceDTO): WorkspaceStatus {
  return ws.status as WorkspaceStatus
}

// A custom emoji wins (an `emoji:<char>` marker), then the repo's /icon URL on its own host so
// WKWebView can load it; else undefined (initials). An unresolved host gets the initials too:
// the rebuild that follows once it resolves redraws it.
function repoAvatarURL(repo: RepoDTO): string | undefined {
  if (repo.avatarEmoji) return `emoji:${repo.avatarEmoji}`
  if (!repo.avatarUrl || !isHostResolved({ projectId: repo.projectId })) return undefined
  return projectAssetURL(repo.projectId, repo.avatarUrl)
}

export function toSidebarWorkspace(ws: WorkspaceDTO): Workspace {
  return {
    // Every field present, never conditionally spread: applyWorkspaceDTO merges `{...w, ...ws}`,
    // so an omitted key keeps its previous value, while a cleared field ('' / undefined — a move to
    // root, a PR closing, a Retry clearing heldByPath) must overwrite.
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
    // Every field present, as in toSidebarWorkspace: a folder dragged to the repo root arrives
    // with an empty parentId that must overwrite the previous one.
    id: folder.id,
    repoId: folder.repoId,
    parentId: folder.parentId || undefined,
    name: folder.name,
    order: folder.order ?? 0,
    createdAt: folder.createdAt,
  }
}

/** ChatDTO -> the sidebar's Chat, every field present (as toSidebarFolder): a chat dragged to the
 *  root arrives with an empty parentId that must overwrite the previous one. */
export function toSidebarChat(chat: ChatDTO): Chat {
  return {
    id: chat.id,
    repoId: chat.repoId,
    type: chat.type,
    parentId: chat.parentId || undefined,
    workspaceId: chat.workspaceId || undefined,
    // Undefined, not false, when the cached row predates the field: false asserts "a bubble",
    // while undefined lets deriveRows fall back to the Workspace.owningChatId join.
    ...(chat.ownsWorktree === undefined ? {} : { ownsWorktree: chat.ownsWorktree }),
    title: chat.title,
    order: chat.order ?? 0,
    createdAt: chat.createdAt,
  }
}

/** Missing order sorts last. */
const NO_ORDER = Number.MAX_SAFE_INTEGER

/** The daemon's sibling sort (tree/node.go compareNodes: order, createdAt, id),
 *  so an undragged level is drawn in the sequence a drop index is counted in —
 *  the cache yields by key, and an arrival tiebreak would draw it in id order. */
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

export function toSidebarRepo(
  repo: RepoDTO,
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo {
  const repoWs = sortByPlacement(workspaces.filter((ws) => ws.repoId === repo.id))
  const repoFolders = folders.filter((folder) => folder.repoId === repo.id)
  // The cross-repo guard, and the reason a chat row carries a repoId at all:
  // the entity cache is deliberately cross-repo and long-lived, so an
  // unfiltered read would hand every repo every other repo's chats.
  const repoChats = chats.filter((chat) => chat.repoId === repo.id)
  const defaultWs = repoWs.find((ws) => ws.isDefault)
  const sidebarWorkspaces: Workspace[] = []
  for (const ws of repoWs) {
    if (!ws.isDefault) sidebarWorkspaces.push(toSidebarWorkspace(ws))
  }
  return {
    id: repo.id,
    projectId: repo.projectId,
    // Never conditionally spread: a repo dragged back to index 0 emits `order: 0`, which must
    // overwrite the previous order.
    order: repo.order ?? 0,
    // Same rule: a repo dragged out of a folder emits `folderId: ''`.
    folderId: repo.folderId ?? '',
    name: repo.name,
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    avatarURL: repoAvatarURL(repo),
    workspaces: sidebarWorkspaces,
    // Omitted when the repo has none, keeping a folderless repo's shape unchanged.
    ...(repoFolders.length > 0 ? { folders: repoFolders } : {}),
    // Omitted when none, like `folders`: a repo whose chat seed hasn't landed has no `chats`.
    ...(repoChats.length > 0 ? { chats: repoChats } : {}),
    ...(repo.deleting ? { deleting: true } : {}),
    // The default workspace is the repo header, filtered out of `workspaces`, so its live `working`
    // is lifted onto the repo for the header and context pill to spin its icon.
    ...(defaultWs
      ? {
          defaultWorkspaceId: defaultWs.id,
          defaultBranch: defaultWs.branch,
          defaultWorking: defaultWs.working,
          // Its status is lifted too: adopted protected branches are locked AND
          // default, and mutation gating (isWorkspaceLockedInSidebar) must see
          // the lock even though the default ws is not a tree row.
          defaultWorkspaceStatus: toSidebarStatus(defaultWs),
          // Lifted for the same reason: the default ws is never a member of
          // `workspaces` below, so there is no `Workspace.owningChatId`
          // `deriveRows` can read for it directly — see `Repo`'s own doc.
          defaultOwningChatId: defaultWs.owningChatId ?? '',
        }
      : {}),
    ...(repo.path ? { localPath: repo.path } : {}),
  }
}

// Repos in sidebar order. `order` is per project, so the flat list interleaves projects — fine,
// since the sidebar buckets by projectId and reads each bucket in order. Ties break as the daemon
// does (compareByPlacement, by id), so an undragged level never reshuffles between rebuilds.
export function sortReposByOrder(repos: readonly Repo[]): Repo[] {
  return sortByPlacement(repos)
}

// Groups workspaces under their repos into the nested Repo[] the sidebar renders. Project-agnostic
// on purpose: each repo keeps its own projectId, so the sidebar can hold several projects at once.
export function buildRepoTree(
  repos: RepoDTO[],
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo[] {
  // Sorted here: the entity cache is a key-value store with no order, so an unsorted rebuild would
  // paint the user's arrangement in whatever sequence IndexedDB yields.
  return sortReposByOrder(repos.map((repo) => toSidebarRepo(repo, workspaces, folders, chats)))
}
