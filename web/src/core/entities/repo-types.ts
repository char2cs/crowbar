import type { ChatType, WorkspaceProvisioning } from '@/lib/types'

// The repo tree the selectors in select.ts answer with: today's sidebar shapes,
// built from the daemon's DTOs by lib/store/build-repo-tree.ts.

/** §5's status union; an agent in flight is the separate `working` flag. */
export type WorkspaceStatus =
  'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'

/** A grouping folder: a tree edge only, so a workspace joins one by `folderId`, never
 *  `parentId` (the fork parent, which merge eligibility and the diff base read). */
export interface Folder {
  id: string
  repoId: string
  /** Owning folder id; undefined or '' at the repo root. */
  parentId?: string
  name: string
  order: number
  /** ISO creation time, the daemon's `order` tiebreak. */
  createdAt?: string
}

/** A conversation row. `parentId` is its one tree edge: a chat, a folder, or absent for
 *  the root of `workspaceId`. */
export interface Chat {
  id: string
  repoId: string
  /** Never a signal of workspace ownership (see `ownsWorktree`); undefined on old rows. */
  type?: ChatType
  /** A chat id, a folder id, or undefined/'' for the root of `workspaceId`. */
  parentId?: string
  /** The workspace this chat runs in: a thread carries its parent's, so it proves no ownership. */
  workspaceId?: string
  /** Whether this row owns `workspaceId`'s worktree; carried on the chat so a row's kind
   *  never waits on the Workspace record. Undefined on a row cached before the field. */
  ownsWorktree?: boolean
  title: string
  /** Sibling sort key, shared with folders and workspaces at the same level. */
  order: number
  /** ISO creation time, the daemon's `order` tiebreak. */
  createdAt?: string
}

export interface Workspace {
  id: string
  branch: string
  parentId?: string
  /** Owning sidebar folder; absent at the repo root. */
  folderId?: string
  /** Sibling sort key; absent on older frames, which fall back to arrival order. */
  order?: number
  /** ISO creation time, the daemon's `order` tiebreak. */
  createdAt?: string
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
  /** True while an agent or long-running op is in flight. */
  working?: boolean
  /** From MergeEligibility: whether this workspace can merge into its parent. */
  canMergeLocally?: boolean
  /** Merging into its parent would conflict, which blocks the merge. */
  mergeConflicts?: boolean
  /** Parent branch name when mergeable. */
  parentBranch?: string
  prUrl?: string
  /** On-disk worktree directory. */
  localPath?: string
  /** Holder path of a placeholder: drives its reason and the Detach… action. */
  heldByPath?: string
  /** Whether this workspace has a checkout at all. */
  provisioning?: WorkspaceProvisioning
  /** The chat that owns this workspace, from the daemon: what a create under it names as its
   *  parent. Not the row id of a fork, whose owner draws as its own row. '' or absent
   *  means nothing to place by yet. */
  owningChatId?: string
}

export interface Repo {
  id: string
  projectId?: string
  /** Index within its project's section; absent sorts after the ordered ones. */
  order?: number
  /** Project-home folder the repo header is filed under; undefined or '' for the root. */
  folderId?: string
  name: string
  avatarLabel: string
  avatarColor: string
  avatarURL?: string
  workspaces: Workspace[]
  folders?: Folder[]
  /** Absent means "not seeded yet", never "no chats": chats arrive on their own reseed
   *  loop, and a folded-away repo has none open. */
  chats?: Chat[]
  /** The default (main-worktree) workspace, which is never a `workspaces` row. */
  defaultWorkspaceId?: string
  /** Its branch, reserved by create-input validation. */
  defaultBranch?: string
  /** Its status (protected-branch adoptions are 'locked'). */
  defaultWorkspaceStatus?: WorkspaceStatus
  /** Its owning chat id; '' or absent falls back to a chat claiming the row (`resolveHomeOwnerId`). */
  defaultOwningChatId?: string
  /** Its `working` flag, which spins the repo icon. */
  defaultWorking?: boolean
  /** On-disk repo root, the default workspace's path. */
  localPath?: string
  /** A recorded delete of this repo is in flight (a failed one keeps the mark). */
  deleting?: boolean
}
