import {
  ArrowClockwise,
  ArrowElbowDownRight,
  DownloadSimple,
  Folder,
  GitBranch,
  LinkBreak,
  Lock,
  LockOpen,
  PencilSimpleLine,
  Terminal,
  Trash,
  X,
  type Icon,
} from '@phosphor-icons/react'
import type { useNavigate } from '@tanstack/react-router'
import {
  performCreateFolder,
  performCreateFolderFromChat,
  performCreateHomeFolder,
  performSetWorkspaceLock,
} from '@/components/sidebar/lib/row-actions'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { handleCreate } from '@/components/layout/create-actions'
import { handleTrash, handleTrashRepo } from '@/components/layout/trash-actions'
import { readSelect, selectRepos } from '@/core/entities/select'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { runOperation } from '@/core/entities/run-operation'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { retryProvision } from '@/lib/api/workspace'
import { toast } from '@/features/window/stores/toast-store'

/** The verbs the desktop puts on a row's hover buttons rather than in its menu. */
export type RowButtonVerb = 'detach' | 'retry' | 'remove' | 'new-thread' | 'fork' | 'branch-out'

export type RowMenuActionId =
  | 'rename'
  | 'lock'
  | 'unlock'
  | 'import'
  | 'new-folder'
  | 'new-thread-terminal'
  | 'delete-repo'
  | RowButtonVerb

/** One verb of a sidebar row's menu: the desktop draws `icon`, the phone's native menu `nativeIcon`. */
export interface RowMenuAction {
  id: RowMenuActionId
  label: string
  icon: Icon
  /** The Phosphor name of `icon`, for the phone's native menu. */
  nativeIcon: string
  destructive: boolean
}

/** What a row's verbs do that differs between the desktop tree and the phone. */
export interface RowMenuHandlers {
  navigate: ReturnType<typeof useNavigate>
  rename: (row: SidebarRow) => void
  importBranches: (row: SidebarRow) => void
  /** Starts a fork of `row`; the desktop names it inline, the phone asks first. */
  fork: (row: SidebarRow) => void
  /** Makes the thread `row` a branch; the desktop names it in place, the phone asks first. */
  branchOut: (row: SidebarRow) => void
  /** The phone confirms natively first, so a removal there commits at once instead of draining. */
  removeAtOnce?: boolean
}

const ACTIONS: Record<RowMenuActionId, Omit<RowMenuAction, 'id'>> = {
  rename: {
    label: 'Rename',
    icon: PencilSimpleLine,
    nativeIcon: 'PencilSimpleLine',
    destructive: false,
  },
  lock: { label: 'Lock', icon: Lock, nativeIcon: 'Lock', destructive: false },
  unlock: { label: 'Unlock', icon: LockOpen, nativeIcon: 'LockOpen', destructive: false },
  import: {
    label: 'Import branches',
    icon: DownloadSimple,
    nativeIcon: 'DownloadSimple',
    destructive: false,
  },
  'new-folder': {
    label: 'New folder',
    icon: Folder,
    nativeIcon: 'Folder',
    destructive: false,
  },
  'new-thread-terminal': {
    label: 'New thread in Terminal',
    icon: Terminal,
    nativeIcon: 'Terminal',
    destructive: false,
  },
  'delete-repo': { label: 'Delete Repo', icon: Trash, nativeIcon: 'Trash', destructive: true },
  detach: {
    label: 'Detach…',
    icon: LinkBreak,
    nativeIcon: 'LinkBreak',
    destructive: false,
  },
  retry: {
    label: 'Retry setup',
    icon: ArrowClockwise,
    nativeIcon: 'ArrowClockwise',
    destructive: false,
  },
  'new-thread': {
    label: 'New thread',
    icon: ArrowElbowDownRight,
    nativeIcon: 'ArrowElbowDownRight',
    destructive: false,
  },
  fork: { label: 'Fork branch', icon: GitBranch, nativeIcon: 'GitBranch', destructive: false },
  'branch-out': {
    label: 'Make a branch',
    icon: GitBranch,
    nativeIcon: 'GitBranch',
    destructive: false,
  },
  remove: { label: 'Remove', icon: X, nativeIcon: 'Trash', destructive: true },
}

/** A repo thread owning no worktree, not already moving, can be made a branch of its own. */
export function canBranchOut(row: SidebarRow, movePending: boolean): boolean {
  return row.kind === 'chat' && row.canBranchOut === true && !movePending
}

// The hover buttons a row earns, in the desktop's left-to-right order. A holder leaves Detach as
// the only remedy, a failed provision Retry; no worktree, nothing that needs one; a chat never
// forks but a repo thread can become a branch; the project home is removed via Delete Repo, and
// a locked branch not at all.
export function rowButtonVerbs(
  row: SidebarRow,
  isProjectHome: boolean,
  movePending = false,
): RowButtonVerb[] {
  const verbs: RowButtonVerb[] = []
  const isBranch = row.kind === 'branch'
  if (isBranch && row.workspaceId && row.heldByPath) verbs.push('detach')
  if (isBranch && row.workspaceId && row.isPlaceholder && !row.heldByPath) verbs.push('retry')
  const hasRepoMenu = isProjectHome && Boolean(row.repoIcon)
  const removable = row.kind === 'chat' || row.kind === 'folder' || (isBranch && !row.locked)
  if (!hasRepoMenu && removable) verbs.push('remove')
  if (!row.isPlaceholder) {
    verbs.push('new-thread')
    if (row.kind === 'folder' ? row.ownsWorktree : isBranch) verbs.push('fork')
    if (canBranchOut(row, movePending)) verbs.push('branch-out')
  }
  return verbs
}

/** Whether the branch row `rowId` names a locked workspace. A locked branch is id'd by the chat
 *  that owns its workspace, so the id is translated into the workspace id space first. */
export function rowIsLocked(rowId: string): boolean {
  const repos = readSelect(selectRepos)
  const wsId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  return repos.some((repo) => repo.workspaces.some((w) => w.id === wsId && w.status === 'locked'))
}

// A row's menu, in order. A locked branch keeps its name (no Rename); Lock/Unlock only for a real
// workspace branch with a worktree (the daemon refuses a placeholder's), never the project-home
// checkout; "New thread in Terminal" is left out when the provider a new chat would start under has no terminal.
export function rowMenuActions(
  row: SidebarRow,
  { locked, canStartOnTerminal }: { locked: boolean; canStartOnTerminal: boolean },
): RowMenuAction[] {
  const isProjectHome = row.repoIcon !== undefined
  const ids: RowMenuActionId[] = []
  if (!(row.kind === 'branch' && locked)) ids.push('rename')
  const lockable = row.kind === 'branch' && !isProjectHome && !row.isPlaceholder
  if (lockable) ids.push(locked ? 'unlock' : 'lock')
  if (isProjectHome) ids.push('import')
  if (row.kind === 'branch' || row.kind === 'folder' || row.kind === 'chat') {
    ids.push('new-folder')
    if (canStartOnTerminal) ids.push('new-thread-terminal')
  }
  if (isProjectHome) ids.push('delete-repo')
  return ids.map((id) => ({ id, ...ACTIONS[id] }))
}

// Every desktop verb, menu and hover buttons together: the phone's one long-press menu. The
// destructive verb goes last, as iOS menus put it.
export function rowVerbs(
  row: SidebarRow,
  ctx: { locked: boolean; canStartOnTerminal: boolean; movePending: boolean },
): RowMenuAction[] {
  const ids = [
    ...rowMenuActions(row, ctx).map((a) => a.id),
    ...rowButtonVerbs(row, row.repoIcon !== undefined, ctx.movePending),
  ]
  const ordered = [
    ...ids.filter((id) => !ACTIONS[id].destructive),
    ...ids.filter((id) => ACTIONS[id].destructive),
  ]
  return ordered.map((id) => ({ id, ...ACTIONS[id] }))
}

function retry(row: SidebarRow) {
  const wsId = row.workspaceId
  if (!wsId) return
  // A failure is the operations watcher's to toast and the row's to explain.
  runOperation(
    {
      kind: 'workspace.retry_provision',
      subject: { kind: 'workspace', id: wsId },
      projectId: '',
      label: `Retry ${row.branchName || row.label}`,
    },
    (opId) => retryProvision(wsId, opId),
  )
}

// A home row rides no repo, so it needs the home-scoped create; a chat carries no folder anchor
// of its own, so its New folder roots at '' rather than nesting under the chat.
function createFolderFor(row: SidebarRow): Promise<void> {
  const homeScope = resolveHomeRowScope(row.id)
  const isChat = row.kind === 'chat'
  if (homeScope) return performCreateHomeFolder(homeScope.projectId, isChat ? '' : row.id)
  return isChat ? performCreateFolderFromChat(row.id) : performCreateFolder(row.id)
}

/** Carries out `id` on `row`. */
export function runRowMenuAction(
  id: RowMenuActionId,
  row: SidebarRow,
  handlers: RowMenuHandlers,
): void {
  switch (id) {
    case 'rename':
      return handlers.rename(row)
    case 'lock':
    case 'unlock':
      void performSetWorkspaceLock(row.id, id === 'lock')
      return
    case 'import':
      return handlers.importBranches(row)
    case 'new-folder':
      void createFolderFor(row)
      return
    case 'new-thread-terminal':
      void handleCreate(row.id, 'thread', handlers.navigate, 'terminal')
      return
    case 'delete-repo':
      // handleTrash refuses this row: it resolves to the repo's own default-branch workspace only.
      if (row.repoIcon && !handleTrashRepo(row.repoIcon.repoId)) {
        toast.error(`Can't delete ${row.label} yet`)
      }
      return
    case 'detach':
      useDetachModalStore.getState().open({
        wsId: row.workspaceId ?? '',
        branch: row.branchName || row.label,
        heldByPath: row.heldByPath ?? '',
      })
      return
    case 'retry':
      return retry(row)
    case 'new-thread':
      void handleCreate(row.id, 'thread', handlers.navigate)
      return
    case 'fork':
      return handlers.fork(row)
    case 'branch-out':
      return handlers.branchOut(row)
    case 'remove':
      if (!handleTrash(row.id, handlers.removeAtOnce ? 0 : undefined))
        toast.error(`Can't remove ${row.label}`)
      return
  }
}
