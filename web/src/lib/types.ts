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
}

export interface Prerequisites {
  git: { installed: boolean; version?: string }
  gh: { installed: boolean; authed: boolean }
  glab: { installed: boolean; authed: boolean }
}

// ---------------------------------------------------------------------------
// Canonical DTOs (§5) — camelCase mirrors of the Go `api/v0/dto` contracts.
// These are the single source of truth the IndexedDB entity cache stores and
// the hierarchical read API returns. Additive: the legacy flat types above stay
// in place until the W17/W18 cutover.
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
  lastError: string
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
  /** Worktree dir holding this branch when the workspace is a placeholder
   *  (locked + no localPath). Absent on healthy workspaces. */
  heldByPath?: string
  /** "home" for the project home workspace; absent or "git" for normal git workspaces. */
  kind?: 'git' | 'home'
  /** Sidebar grouping folder this workspace belongs to, or absent for the repo
   *  root. A SEPARATE field from parentId, which stays the fork parent. */
  folderId?: string
  /** Dense sibling sort key within its level. Absent on frames from a daemon
   *  that predates ordering. */
  order?: number
  /** The chat row that OWNS this workspace — the row the daemon resolves a
   *  placement against, so it is what a create under this workspace must name
   *  as its parent. The daemon always sends the key (`""` when it could resolve
   *  none); absent only on a row cached before the field existed. */
  owningChatId?: string
}

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
}

export interface FolderDTO {
  id: string
  repoId: string
  projectId: string
  /** A workspace id, another folder id, or absent for the repo root. Note this
   *  is the SIDEBAR parent — a folder has no branch, so it is never a fork
   *  parent. */
  parentId?: string
  name: string
  /** Dense sibling sort key. Folders and workspaces share one sibling space, so
   *  this is compared against WorkspaceDTO.order at the same level. */
  order: number
  /** Tombstone marker on a broadcast frame: '' (or absent) for a live folder,
   *  'deleted' for a removal frame. Read-path DTOs leave it empty. */
  status?: string
}

/**
 * A row's own kind in the sidebar forest (Go's `domain.ChatType`). One aggregate
 * carries all four: `folder` rows are what `.../chats/folders` serves, `branch`
 * rows ARE the workspaces they own (a locked branch, a repo home, a project
 * home), and `chat` is an ordinary conversation.
 */
export type ChatType = 'chat' | 'branch' | 'folder' | 'workflow'

/**
 * A conversation row of the sidebar forest — design spec §3.1's `chat` kind,
 * the one row type the tree was built around and never emitted.
 *
 * Two facts decide which of §3.1's two chat rows this is: a WORKTREE chat owns
 * `workspaceId`, a BUBBLE chat leaves it empty and borrows an ancestor's ground.
 * Neither is a Recents-only concept — both are tree rows on equal footing with
 * branches and folders.
 *
 * `repoId`/`projectId` are stamped from the URL the row was read through, as
 * `FolderDTO`'s are: no chat row carries a repo id on the wire, and none should
 * (api/internal/app/usecases/chat/repo_scope.go — a row's repo is the workspace
 * its cwd walk lands on, derived server-side, never stored).
 */
export interface ChatDTO {
  id: string
  repoId: string
  projectId: string
  /** This row's own kind in the sidebar forest (domain.ChatType). A `branch`
   *  row IS the workspace it owns — a locked branch, a repo home, a project
   *  home — which is what lets a client tell one apart from an ordinary chat
   *  inside the same repo-scoped list. Optional only because a row cached
   *  before the daemon emitted it carries none. */
  type?: ChatType
  /** The workspace this chat OWNS, or '' for a bubble that owns none. */
  workspaceId: string
  /** Another CHAT (this one is a thread of it), a FOLDER, or '' for the root of
   *  whatever workspace `workspaceId` names. */
  parentId?: string
  title: string
  /** Dense sibling sort key, SHARED with folders and workspaces at the same
   *  level — compared against FolderDTO.order / WorkspaceDTO.order. */
  order: number
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

/** Which agent wrote a review message, and out of which chat.
 *
 *  Both are omitempty on the wire and ABSENT far more often than present: every
 *  human message has neither, and so does every agent message written before
 *  attribution existed. A client renders exactly as it did before this pair when
 *  they are missing, and resolves them against the providers and chats it already
 *  holds when they are not — never branching on a particular id's value. */
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

/** The root message's attribution rides on the THREAD, alongside its body, author
 *  and agent flag — the root is flattened onto the thread rather than appearing in
 *  `replies`, so without it an agent-opened thread would render attributed on every
 *  reply and anonymous on the finding itself. */
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
  /** Tombstone flag on a broadcast frame: the thread was deleted, drop it. */
  deleted?: boolean
}

export interface TerminalSessionDTO {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  profileId: string
  status: 'active' | 'detached' | 'suspended' | 'ended'
  /** Process exit code; only present on "ended" frames where the exit code is known (>=0). */
  exitCode?: number
  createdAt: string
  endedAt: string | null
}
