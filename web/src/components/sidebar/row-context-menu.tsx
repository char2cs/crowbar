import { useEffect, type RefObject } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ArrowsOutCardinal, Folder, GitBranch, XCircle } from '@phosphor-icons/react'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import {
  canBranchOut,
  rowIsLocked,
  rowMenuActions,
  runRowMenuAction,
} from '@/components/sidebar/lib/row-menu'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { providerCanStartOnTerminal } from '@/features/agent/api/agent-providers-api'
import { handleCreate } from '@/components/layout/create-actions'
import { useKillAgentMenuItems } from '@/features/agent/hooks/use-kill-agent-menu-items'
import { moveTargets } from '@/components/sidebar/lib/move-targets'
import { performCancelMove, performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { readSelect, selectAgentChat } from '@/core/entities/select'

const DESTRUCTIVE =
  'text-destructive data-highlighted:bg-destructive/10 data-highlighted:text-destructive dark:data-highlighted:bg-destructive/20'

interface SidebarRowContextMenuProps {
  treeRef: RefObject<HTMLElement | null>
  /** Every visible row, to look up kind/parentId by id. */
  rows: SidebarRow[]
  /** Opens the restored RepoImportDialog for the project-home row `repoRowId`. */
  onImport: (repoRowId: string) => void
}

interface MenuData {
  row: SidebarRow
  /** Read from the repos at open time: `SidebarRow` carries no `locked` field. */
  locked: boolean
  /** The chat waits to move to another workspace, read off its entity at open time. */
  movePending: boolean
  /** A repo thread owning no worktree, not already moving: it can become a branch. */
  canBranch: boolean
}

// The sidebar's right-click menu. A sibling of the tree listening for native `contextmenu`, so
// opening it re-renders only itself, not every row. It acts on the one row under the pointer
// (found via `data-sidebar-row-id`); there is no multiselect.
export function SidebarRowContextMenu({ treeRef, rows, onImport }: SidebarRowContextMenuProps) {
  const menu = useContextMenu<MenuData>()
  const { openAt } = menu
  const navigate = useNavigate()
  // "New thread in Terminal" is absent, not disabled, when the provider a new chat would start
  // under has no terminal (spec 2.5). No resolved provider is permissive, like the Thread button,
  // which leaves the refusal to click time.
  const enabledProviderCanStartOnTerminal = useAgentProvidersStore((s) =>
    providerCanStartOnTerminal(s.providers.find((p) => p.enabled)),
  )

  const killItems = useKillAgentMenuItems(menu.data?.row.id)

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const resolveMenuData = (rowId: string): MenuData | null => {
      const row = rows.find((r) => r.id === rowId)
      if (!row) return null
      const locked = rowIsLocked(rowId)
      const movePending = !!readSelect((s) => selectAgentChat(s, rowId))?.pendingMove
      return { row, locked, movePending, canBranch: canBranchOut(row, movePending) }
    }
    const onContextMenu = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const data = resolveMenuData(rowId)
      if (!data) return
      e.preventDefault()
      openAt({ x: e.clientX, y: e.clientY }, data)
    }
    // The row's "..." button opens this same menu under itself, so one row has one menu. Capture
    // phase, not bubble: the row's trailing buttons stop propagation on the way up, and capture
    // runs before the row ever sees the click.
    const onRepoMenuClick = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const trigger = e.target.closest<HTMLElement>('[data-control="repo-menu"]')
      if (!trigger) return
      const el = trigger.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const data = resolveMenuData(rowId)
      if (!data) return
      e.preventDefault()
      e.stopPropagation()
      const rect = trigger.getBoundingClientRect()
      openAt({ x: rect.left, y: rect.bottom + 4 }, data)
    }
    tree.addEventListener('contextmenu', onContextMenu)
    tree.addEventListener('click', onRepoMenuClick, true)
    return () => {
      tree.removeEventListener('contextmenu', onContextMenu)
      tree.removeEventListener('click', onRepoMenuClick, true)
    }
  }, [treeRef, rows, openAt])

  if (!menu.data) return null
  const { row, locked, movePending, canBranch } = menu.data
  // Rename starts the same inline editor double-click does; the store knows which instance draws it.
  const handlers = {
    navigate,
    rename: (target: SidebarRow) => treeUiStore.getState().startRenaming(target.id),
    importBranches: (target: SidebarRow) => onImport(target.id),
    fork: (target: SidebarRow) => void handleCreate(target.id, 'workspace', navigate),
    branchOut: (target: SidebarRow) => treeUiStore.getState().startBranching(target.id),
  }
  const items: ContextMenuItem[] = rowMenuActions(row, {
    locked,
    canStartOnTerminal: enabledProviderCanStartOnTerminal,
  }).flatMap((action): ContextMenuItem[] => {
    const Icon = action.icon
    const item: ContextMenuItem = {
      id: action.id,
      label: action.label,
      icon: <Icon />,
      onClick: () => runRowMenuAction(action.id, row, handlers),
      ...(action.destructive && { className: DESTRUCTIVE }),
    }
    if (!action.destructive) return [item]
    const separator = {
      id: `${action.id}-separator`,
      separator: true,
      label: '',
      onClick: () => {},
    }
    return [separator, item]
  })
  // Kill agent sits with the destructive group, ahead of its first separator; Move to… before it.
  const destructiveAt = items.findIndex((i) => i.separator)
  items.splice(
    destructiveAt < 0 ? items.length : destructiveAt,
    0,
    ...moveItem(row, rows),
    ...(canBranch ? [branchOutItem(row)] : []),
    ...(movePending ? [cancelMoveItem(row)] : []),
    ...killItems,
  )

  return (
    <ContextMenu isOpen={menu.isOpen} items={items} position={menu.position} onClose={menu.close} />
  )
}

// Moving a row without a pointer: each folder, repo or branch a drag could drop it into,
// committed as that drop. A row with nowhere to go gets no entry.
function moveItem(row: SidebarRow, rows: readonly SidebarRow[]): ContextMenuItem[] {
  const targets = moveTargets(row, rows)
  if (targets.length === 0) return []
  const items = targets.map((target): ContextMenuItem => ({
    id: `move-to-${target.id}`,
    // A branch row's label is its owning chat's title; the place is the branch.
    label: (!target.repoIcon && target.branchName) || target.label,
    icon: target.kind === 'folder' ? <Folder /> : <GitBranch />,
    onClick: () => void performSidebarDrop([row], target, 'into'),
  }))
  return [
    { id: 'move-to', label: 'Move to…', icon: <ArrowsOutCardinal />, onClick: () => {}, items },
  ]
}

// A chat dropped into another workspace while it worked waits for its turn to end; this keeps it.
function cancelMoveItem(row: SidebarRow): ContextMenuItem {
  return {
    id: 'cancel-move',
    label: 'Cancel move',
    icon: <XCircle />,
    onClick: () => void performCancelMove(row),
  }
}

// A thread made a branch of its own, cut from its folder's HEAD once its turn ends.
function branchOutItem(row: SidebarRow): ContextMenuItem {
  return {
    id: 'branch-out',
    label: 'Make a branch',
    icon: <GitBranch />,
    onClick: () => treeUiStore.getState().startBranching(row.id),
  }
}
