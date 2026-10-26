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
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { retryProvision } from '@/lib/api/workspace'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'

/** The verbs the desktop puts on a row's hover buttons rather than in its menu. */
export type RowButtonVerb = 'detach' | 'retry' | 'remove' | 'new-thread' | 'fork'

export type RowMenuActionId =
  | 'rename'
  | 'lock'
  | 'unlock'
  | 'import'
  | 'new-folder'
  | 'new-thread-terminal'
  | 'delete-repo'
  | RowButtonVerb

/** One verb of a sidebar row's menu: the desktop draws `icon`, the phone's native menu `symbol`. */
export interface RowMenuAction {
  id: RowMenuActionId
  label: string
  icon: Icon
  /** SF Symbol name for the phone's native menu. */
  symbol: string
  destructive: boolean
}

/** What a row's verbs do that differs between the desktop tree and the phone. */
export interface RowMenuHandlers {
  navigate: ReturnType<typeof useNavigate>
  rename: (row: SidebarRow) => void
  importBranches: (row: SidebarRow) => void
  /** Starts a fork of `row`; the desktop names it inline, the phone asks first. */
  fork: (row: SidebarRow) => void
  /** The phone confirms natively first, so a removal there commits at once instead of draining. */
  removeAtOnce?: boolean
}

const ACTIONS: Record<RowMenuActionId, Omit<RowMenuAction, 'id'>> = {
  rename: { label: 'Rename', icon: PencilSimpleLine, symbol: 'pencil', destructive: false },
  lock: { label: 'Lock', icon: Lock, symbol: 'lock', destructive: false },
  unlock: { label: 'Unlock', icon: LockOpen, symbol: 'lock.open', destructive: false },
  import: {
    label: 'Import branches',
    icon: DownloadSimple,
    symbol: 'square.and.arrow.down',
    destructive: false,
  },
  'new-folder': {
    label: 'New folder',
    icon: Folder,
    symbol: 'folder.badge.plus',
    destructive: false,
  },
  'new-thread-terminal': {
    label: 'New thread in Terminal',
    icon: Terminal,
    symbol: 'apple.terminal',
    destructive: false,
  },
  'delete-repo': { label: 'Delete Repo', icon: Trash, symbol: 'trash', destructive: true },
  detach: {
    label: 'Detach…',
    icon: LinkBreak,
    symbol: 'personalhotspot.slash',
    destructive: false,
  },
  retry: {
    label: 'Retry setup',
    icon: ArrowClockwise,
    symbol: 'arrow.clockwise',
    destructive: false,
  },
  'new-thread': {
    label: 'New thread',
    icon: ArrowElbowDownRight,
    symbol: 'arrow.turn.down.right',
    destructive: false,
  },
  fork: { label: 'Fork branch', icon: GitBranch, symbol: 'arrow.branch', destructive: false },
  remove: { label: 'Remove', icon: X, symbol: 'trash', destructive: true },
}

/**
 * The hover buttons a row earns, in the desktop's left-to-right order (SidebarRowActions draws
 * them from this). A holder leaves Detach as the only remedy, a failed provision Retry; a row
 * with no worktree offers nothing that needs one; a chat never forks; the project home's
 * removal is its menu's Delete Repo, and a locked branch is not removable.
 */
export function rowButtonVerbs(row: SidebarRow, isProjectHome: boolean): RowButtonVerb[] {
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
  }
  return verbs
}

/** Whether the branch row `rowId` names a locked workspace. A locked branch is id'd by the chat
 *  that owns its workspace, so the id is translated into the workspace id space first. */
export function rowIsLocked(rowId: string): boolean {
  const repos = useSidebarStore.getState().repos
  const wsId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  return repos.some((repo) => repo.workspaces.some((w) => w.id === wsId && w.status === 'locked'))
}

/**
 * A row's menu, in order. A locked branch keeps its name (no Rename); Lock/Unlock is for a real
 * workspace branch, never the project-home row (the checkout that must stay put); "New thread in
 * Terminal" is left out when the provider a new chat would start under has no terminal.
 */
export function rowMenuActions(
  row: SidebarRow,
  { locked, canStartOnTerminal }: { locked: boolean; canStartOnTerminal: boolean },
): RowMenuAction[] {
  const isProjectHome = row.repoIcon !== undefined
  const ids: RowMenuActionId[] = []
  if (!(row.kind === 'branch' && locked)) ids.push('rename')
  if (row.kind === 'branch' && !isProjectHome) ids.push(locked ? 'unlock' : 'lock')
  if (isProjectHome) ids.push('import')
  if (row.kind === 'branch' || row.kind === 'folder' || row.kind === 'chat') {
    ids.push('new-folder')
    if (canStartOnTerminal) ids.push('new-thread-terminal')
  }
  if (isProjectHome) ids.push('delete-repo')
  return ids.map((id) => ({ id, ...ACTIONS[id] }))
}

/**
 * Every verb a row has on the desktop, menu and hover buttons together: what the phone's one
 * long-press menu offers. The destructive verb goes last, as iOS menus put it.
 */
export function rowVerbs(
  row: SidebarRow,
  ctx: { locked: boolean; canStartOnTerminal: boolean },
): RowMenuAction[] {
  const ids = [
    ...rowMenuActions(row, ctx).map((a) => a.id),
    ...rowButtonVerbs(row, row.repoIcon !== undefined),
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
  void retryProvision(wsId).catch((err: unknown) => {
    toast.show({
      message: `Couldn't set up ${row.branchName || row.label}`,
      description: actionErrorMessage(err, String(err)),
      type: 'error',
      key: `retry-provision-${wsId}`,
    })
  })
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
    case 'remove':
      if (!handleTrash(row.id, handlers.removeAtOnce ? 0 : undefined))
        toast.error(`Can't remove ${row.label}`)
      return
  }
}
