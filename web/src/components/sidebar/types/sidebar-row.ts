import type { WorkspaceStatus } from '@/core/entities/repo-types'
import type { OperationState } from '@/core/entities/operations'
import type { Problem } from '@/lib/errors/problem'

export type SidebarRowKind = 'chat' | 'branch' | 'folder' | 'workflow'

export interface SidebarRow {
  id: string
  kind: SidebarRowKind
  parentId: string | null
  order: number
  /** ISO creation time — the daemon's `order` tiebreak, so a tied level is
   *  drawn in the sequence a drop index is counted (`compareSidebarRows`). */
  createdAt?: string
  label: string
  labelProvisional?: boolean
  ownsWorktree: boolean
  workspaceId: string | null
  /** Chat rows only: the thread sits in a repo, so Make a branch can cut one from its folder's
   *  HEAD (what the daemon's branch-out accepts). False for a project-home chat. */
  canBranchOut?: boolean
  working: boolean
  hasView: boolean
  branchName?: string
  /** Lines added/deleted vs the fork parent, for a branch row's second line. Only on a branch
   *  row owning a real workspace; absent (not zero) means no diff is known yet. */
  added?: number
  deleted?: number
  /** The owned workspace is protected/locked — the Lock glyph's signal. */
  locked?: boolean
  /** The owned workspace's daemon status ('new', 'locked', PR states), drawn as the same icon
   *  the workspace switcher uses. Present under the same condition as `added`. */
  status?: WorkspaceStatus
  /** The owned workspace has no on-disk worktree; gates every verb that needs a directory
   *  (Thread/Fork). Absent is never treated as true. */
  isPlaceholder?: boolean
  /** The 'unprovisioned' half of `isPlaceholder`: a worktree was tried and failed. The warning
   *  glyph and toast key off this, not `isPlaceholder` — the repo's own checkout ('own-checkout')
   *  is a normal resting state and Retry would refuse it. */
  needsProvisioning?: boolean
  /** The worktree holding this row's branch when it has none of its own. Non-empty exactly
   *  when Detach… applies — including the repo's own checkout (§3.5), where `isPlaceholder`
   *  is false, so the control gates on this. */
  heldByPath?: string
  /** The no-worktree explanation the glyph titles itself with; '' or absent when none. */
  placeholderReason?: string
  /** The repo's identity, only on its header row (whose parent may be a home folder): the icon
   *  editor's REST base and mark, and the drop policy's sign this row is placed on the Repository
   *  aggregate. Absent until the repo's projectId seeds: generic glyph, no repo drag. */
  repoIcon?: {
    repoId: string
    projectId: string
    name: string
    avatarLabel: string
    avatarColor: string
    avatarURL?: string
  }
  /** Only on a row drawn from an operation whose entity hasn't arrived: a spinner while it runs,
   *  else its reason and a Dismiss. No real chat/workspace exists at `id` yet. */
  operation?: {
    opId: string
    state: OperationState
    problem: Problem | null
  }
  /** The sidebar's naming input for a create not yet sent (the tree UI store's `draft`). */
  draft?: true
  /** On a real row held in the removal tray (attachRemovalState matches a branch row by
   *  `workspaceId`): the row draws its countdown and Keep in place. Never alongside
   *  `operation` — no removal names an entity that hasn't arrived. */
  removal?: {
    entryId: string
    /** Null only for 'repo'/'project' entries, which never attach to a row. */
    deadlineAt: number | null
    extra: number
  }
}
