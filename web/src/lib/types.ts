// web/src/lib/types.ts
export interface WorkspacePayload {
  id: string
  projectId: string
  repoId: string
  branch: string
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  toolCalls?: number
  durationSec?: number
}

export interface Project {
  id: string
  name: string
  path: string
  lastActivity: Date
  /** Dense index in the sidebar. Absent on frames from a daemon that predates
   *  ordering, in which case the list keeps arrival order. */
  order?: number
  /** The icon proxy URL, present only when the project has an uploaded image.
   *  Absent means "no image"; see avatarEmoji for the other custom state, and
   *  the Library glyph for the default. */
  avatarUrl?: string
  /** The emoji icon, rendered directly. Wins over avatarUrl — the daemon clears
   *  one when the other is set, so both are never live at once. */
  avatarEmoji?: string
  /** The daemon this project lives on ('local' or a paired host's id). Stamped
   *  by whoever read it off that host — the wire DTO never carries it. */
  hostId: string
}

export interface Prerequisites {
  git: { installed: boolean; version?: string }
  gh: { installed: boolean; authed: boolean }
  glab: { installed: boolean; authed: boolean }
}

// ---------------------------------------------------------------------------
// Canonical DTOs (§5): camelCase mirrors of Go's `api/v0/dto`, as cached and as read.
// ---------------------------------------------------------------------------

export type WorkspaceStatusDTO =
  'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'

export interface WorkspaceDTO {
  id: string
  repoId: string
  projectId: string
  branch: string
  parentId: string
  forkPointSha: string
  status: WorkspaceStatusDTO
  working: boolean
  isDefault?: boolean
  added: number
  deleted: number
  mergeStrategy: string
  canMergeLocally: boolean
  mergeConflicts: boolean
  parentBranch: string
  prUrl: string
  prTitle: string
  prTargetBranch: string
  /** On-disk worktree directory for this workspace (e.g. /home/user/project). */
  localPath?: string
  /** Worktree dir holding this branch when the workspace is a placeholder.
   *  Absent on healthy workspaces. */
  heldByPath?: string
  /** What stands behind localPath — see WorkspaceProvisioning. */
  provisioning: WorkspaceProvisioning
  /** "home" for the project home workspace; absent or "git" for normal git workspaces. */
  kind?: 'git' | 'home'
  /** Sidebar grouping folder this workspace belongs to, or absent for the repo
   *  root. A SEPARATE field from parentId, which stays the fork parent. */
  folderId?: string
  /** Dense sibling sort key within its level. Absent on frames from a daemon
   *  that predates ordering. */
  order?: number
  /** ISO creation time — the daemon's `order` tiebreak (tree.compareNodes), so
   *  an undragged level draws in the sequence a placement index counts. */
  createdAt?: string
  /** The chat row owning this workspace, which the daemon resolves placements against — what a
   *  create under it names as parent. Always sent ('' when unresolved); absent only on old cache. */
  owningChatId?: string
}

// What stands behind a workspace's localPath, recorded by the daemon, never inferred from the
// path: a managed worktree (`provisioned`), none yet because another checkout holds the branch
// (`placeholder`), or the user's own checkout adopted in place (`shared`).
export type WorkspaceProvisioning = 'provisioned' | 'placeholder' | 'shared'

export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
  avatarUrl: string // proxied /v0/projects/:p/repos/:r/icon endpoint
  avatarEmoji: string
  /** Dense index within its project's sidebar section. Absent on frames from a
   *  daemon that predates ordering. */
  order?: number
  /** Project-home folder this repo's entry is filed under, absent (root) on
   *  frames from a daemon that predates repo placement. */
  folderId?: string
  /** The delete intent is recorded; the daemon keeps the row until the delete
   *  finishes, and resumes one a restart interrupted. */
  deleting?: boolean
}

export interface FolderDTO {
  id: string
  repoId: string
  projectId: string
  /** The SIDEBAR parent: a workspace id, another folder id, or absent for the repo root. A
   *  folder has no branch, so it is never a fork parent. */
  parentId?: string
  name: string
  /** Dense sibling sort key. Folders and workspaces share one sibling space, so
   *  this is compared against WorkspaceDTO.order at the same level. */
  order: number
  /** ISO creation time, the `order` tiebreak — see WorkspaceDTO.createdAt. */
  createdAt?: string
  /** Tombstone marker on a broadcast frame: '' (or absent) for a live folder,
   *  'deleted' for a removal frame. Read-path DTOs leave it empty. */
  status?: string
}

// A Chat row's kind (domain.ChatType). Locked branches and homes are workspace nodes and folders
// are FolderDTO entities; only the folder routes answer folder-typed rows, decoded in api/wire.ts.
export type ChatType = 'chat' | 'workflow'

// A conversation row of the sidebar forest (spec §3.1): a worktree chat owns `workspaceId`, a
// bubble leaves it empty and borrows an ancestor's ground. `repoId`/`projectId` are stamped from
// the URL it was read through — the wire carries none (the daemon derives a chat's repo).
export interface ChatDTO {
  id: string
  repoId: string
  projectId: string
  /** The row's kind, 'chat' or 'workflow'; optional because an older cached row carries none. */
  type?: ChatType
  /** The workspace this chat runs in. NOT proof of ownership — a thread carries
   *  its parent's — see {@link ChatDTO.ownsWorktree}. '' for a row that has no
   *  worktree ground at all. */
  workspaceId: string
  /** Whether this row owns `workspaceId`'s worktree, read off the same wire row
   *  (`worktree.owningChatId === id`) so the chat draws as a workspace even while its
   *  WorkspaceDTO lands a frame later. Absent on old cache; deriveRows then joins on owningChatId. */
  ownsWorktree?: boolean
  /** Another CHAT (this one is a thread of it), a FOLDER, or '' for the root of
   *  whatever workspace `workspaceId` names. */
  parentId?: string
  title: string
  /** Dense sibling sort key, SHARED with folders and workspaces at the same
   *  level — compared against FolderDTO.order / WorkspaceDTO.order. */
  order: number
  /** ISO creation time, the `order` tiebreak — see WorkspaceDTO.createdAt. */
  createdAt?: string
}

export interface ProjectDTO {
  id: string
  name: string
  path: string
  status: string // "" | "deleted"
  lastActivity: string
  /** Dense index in the sidebar. Absent on frames from a daemon that predates
   *  ordering. */
  order?: number
  /** The icon proxy URL, present only when the project has an uploaded image.
   *  Absent means "no image"; see avatarEmoji for the other custom state, and
   *  the Library glyph for the default. */
  avatarUrl?: string
  /** The emoji icon, rendered directly. Wins over avatarUrl — the daemon clears
   *  one when the other is set, so both are never live at once. */
  avatarEmoji?: string
}

/** Which agent wrote a review message, out of which chat. Omitempty and usually absent (every
 *  human message, older agent messages): render as unattributed then, and never branch on an id. */
interface AgentAttribution {
  providerId?: string
  chatId?: string
}

export interface ThreadReplyDTO extends AgentAttribution {
  id: string
  threadId: string
  body: string
  author: string
  isAgent: boolean
  createdAt: string
}

/** The root message's attribution rides on the thread, since the root is flattened onto it
 *  rather than appearing in `replies`. */
export interface ThreadDTO extends AgentAttribution {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  filePath: string
  line: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  /** Real id of the root comment (Messages[0]); lets the client edit the root. */
  messageId: string
  body: string
  author: string
  isAgent: boolean
  resolved: boolean
  createdAt: string
  replies: ThreadReplyDTO[]
}

export interface TerminalSessionDTO {
  id: string
  /** The chat that owns this session — the frame's only id and the lifecycle stream's topic key.
   *  A session belongs to its chat, not its worktree, so there is nothing else to key by. */
  chatId: string
  profileId: string
  status: 'active' | 'detached' | 'suspended' | 'ended'
  /** Process exit code; only present on "ended" frames where the exit code is known (>=0). */
  exitCode?: number
  createdAt: string
  endedAt: string | null
}
