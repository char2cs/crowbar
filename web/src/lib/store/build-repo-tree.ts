import { assetURL } from '@/lib/api'
import type { Chat, Folder, Repo, Workspace, WorkspaceStatus } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

// Re-export the canonical §5 DTOs so existing importers (workspace-list, tests)
// keep a single source of truth instead of a divergent local shape.
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

// §5/§6: status now passes straight through — locked / pr-conflicts / deleted
// are first-class WorkspaceStatus values, no longer derived from removed
// agentRunning/locked/hasConflicts overlays. `working` is a separate flag the
// sidebar renders as an in-progress indicator, not a status.
export function toSidebarStatus(ws: WorkspaceDTO): WorkspaceStatus {
  return ws.status as WorkspaceStatus
}

// repoAvatar resolves the sidebar avatar source from the §5 RepoDTO: a custom
// emoji takes precedence (rendered as an `emoji:<char>` marker the sidebar
// understands), otherwise the proxied /icon URL (resolved through assetURL so
// WKWebView can load it cross-origin), otherwise undefined → initials fallback.
function repoAvatarURL(repo: RepoDTO): string | undefined {
  if (repo.avatarEmoji) return `emoji:${repo.avatarEmoji}`
  if (repo.avatarUrl) return assetURL(repo.avatarUrl)
  return undefined
}

export function toSidebarWorkspace(ws: WorkspaceDTO): Workspace {
  return {
    // EVERY field is present, never conditionally spread. applyWorkspaceDTO
    // merges a live frame with {...w, ...ws}, and an omitted key cannot clear
    // anything — it silently keeps the previous value. That used to be masked
    // by the whole-tree rebuild every frame also triggered (which rebuilt each
    // row from scratch); now that a frame is merged incrementally, a CLEARED
    // field arrives here as '' / undefined and has to overwrite. Concretely:
    // reparenting a workspace to the repo root, its parent branch going away
    // with it, a PR closing, a Retry clearing heldByPath or lastError, a move
    // out of a folder.
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
    lastError: ws.lastError ?? '',
    heldByPath: ws.heldByPath ?? '',
    age: '',
    localPath: ws.localPath || undefined,
  }
}

export function toSidebarFolder(folder: FolderDTO): Folder {
  return {
    // Every field present, never conditionally spread — same rule as
    // toSidebarWorkspace: applyFolderDTO merges a live frame with {...f, ...folder},
    // and an omitted key silently keeps the previous value instead of clearing
    // it. Concretely, dragging a folder out to the repo root arrives as an empty
    // parentId and has to overwrite the id it used to hold.
    id: folder.id,
    repoId: folder.repoId,
    parentId: folder.parentId || undefined,
    name: folder.name,
    order: folder.order ?? 0,
  }
}

/** `ChatDTO` -> the sidebar's own `Chat`. Every field present, never
 *  conditionally spread — same rule as `toSidebarFolder` above: a chat dragged
 *  out to the root arrives as an empty parentId and has to overwrite the id it
 *  used to hold. */
export function toSidebarChat(chat: ChatDTO): Chat {
  return {
    id: chat.id,
    repoId: chat.repoId,
    parentId: chat.parentId || undefined,
    workspaceId: chat.workspaceId || undefined,
    title: chat.title,
    order: chat.order ?? 0,
  }
}

export function toSidebarRepo(
  repo: RepoDTO,
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo {
  const repoWs = workspaces.filter((ws) => ws.repoId === repo.id)
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
    // Never conditionally spread: like a workspace's, this field has to be able
    // to overwrite. A repo dragged back to index 0 emits `order: 0`, and an
    // omitted key cannot clear the 3 it used to hold.
    order: repo.order ?? 0,
    name: repo.name,
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    avatarURL: repoAvatarURL(repo),
    workspaces: sidebarWorkspaces,
    // Omitted entirely when the repo has none, so a repo built from a backend
    // that does not emit folders yet is byte-identical to what it was before.
    ...(repoFolders.length > 0 ? { folders: repoFolders } : {}),
    // Same rule as `folders` above — omitted entirely when the repo has none,
    // so a repo whose chat seed has not landed is byte-identical to what it was
    // before this pipeline existed.
    ...(repoChats.length > 0 ? { chats: repoChats } : {}),
    // The default (repo-home) workspace is filtered out of `workspaces` above —
    // it is the repo header, not a tree row — so its live `working` overlay would
    // be dropped with it. Lift it onto the repo as defaultWorking so the header
    // and the context pill can spin the repo's icon during an agent turn.
    ...(defaultWs
      ? {
          defaultWorkspaceId: defaultWs.id,
          defaultBranch: defaultWs.branch,
          defaultWorking: defaultWs.working,
          // Its status is lifted too: adopted protected branches are locked AND
          // default, and mutation gating (isWorkspaceLockedInSidebar) must see
          // the lock even though the default ws is not a tree row.
          defaultWorkspaceStatus: toSidebarStatus(defaultWs),
        }
      : {}),
    ...(repo.path ? { localPath: repo.path } : {}),
  }
}

/** Missing order sorts last, and ties keep arrival order via the index tiebreak
 *  — the same rule buildSidebarTree applies to a level of workspaces. */
const NO_ORDER = Number.MAX_SAFE_INTEGER

/**
 * Repos in sidebar order.
 *
 * `order` is an index WITHIN a project, so the flat list this returns is every
 * project's sequence interleaved — which is exactly what the sidebar consumes,
 * because it buckets by `projectId` and reads each bucket in array order. Where
 * a bucket lands in the flat array carries nothing; the project list decides
 * that (see applyPlacement).
 *
 * The arrival tiebreak is explicit rather than leaning on Array.sort's
 * stability, so a level nobody has dragged yet — every repo still holding 0 —
 * keeps the order it was delivered in instead of reshuffling per frame.
 */
export function sortReposByOrder(repos: readonly Repo[]): Repo[] {
  const arrival = new Map(repos.map((repo, i) => [repo.id, i]))
  return [...repos].sort(
    (a, b) =>
      (a.order ?? NO_ORDER) - (b.order ?? NO_ORDER) ||
      (arrival.get(a.id) ?? 0) - (arrival.get(b.id) ?? 0),
  )
}

// buildRepoTree groups the workspace list under their repos to produce the
// nested Repo[] the sidebar renders.
//
// It is deliberately PROJECT-AGNOSTIC: it groups whatever repos it is handed and
// tags each with its own `projectId`, so the sidebar can hold several projects
// at once. The single-active-project filter that used to live here
// (`buildScopedRepoTree`) has moved to lib/store/project-visibility.ts, where it
// is expressed as "which projects are VISIBLE" rather than "which one is
// active" — see readVisibleRepoTree.
export function buildRepoTree(
  repos: RepoDTO[],
  workspaces: WorkspaceDTO[],
  folders: Folder[] = [],
  chats: Chat[] = [],
): Repo[] {
  // Ordered here rather than left to the caller: the entity cache this is
  // usually built from is a key-value store with no order of its own, so an
  // unsorted rebuild would paint the user's arrangement in whatever sequence
  // IndexedDB happened to yield.
  return sortReposByOrder(repos.map((repo) => toSidebarRepo(repo, workspaces, folders, chats)))
}
