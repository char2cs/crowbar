import { useEffect, type RefObject } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { useSidebarInlineRenameStore } from '@/lib/store/sidebar-inline-rename'
import { rowIsLocked, rowMenuActions, runRowMenuAction } from '@/components/sidebar/lib/row-menu'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { providerCanStartOnTerminal } from '@/features/agent/api/agent-api'
import { handleCreate } from '@/components/layout/create-actions'
import { useKillAgentMenuItems } from '@/features/agent/hooks/use-kill-agent-menu-items'

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
  /** Read from `useSidebarStore` at open time — `SidebarRow` carries no
   *  `locked` field yet, so this can't come from the `rows` prop. */
  locked: boolean
}

/**
 * The sidebar's right-click menu — rename, lock/unlock, branch import,
 * "New folder", and "New thread in Terminal", the verbs Task 8's
 * unification (and this fix) left with no home on `SidebarRow`'s four-prop
 * surface.
 *
 * A SIBLING of the tree, listening for a native `contextmenu` event on
 * `treeRef.current` rather than a hook inside the tree: with the open/closed
 * state inside the tree component, opening this popup re-renders every row
 * to draw a menu that isn't part of the tree at all (the deleted
 * `row-context-menu.tsx` measured this before landing on the sibling
 * design). Out here it re-renders itself.
 *
 * No multiselect: this task doesn't touch the drag/selection system, so a
 * right-click always acts on exactly the one row under the pointer — found
 * via `data-sidebar-row-id`, not the drag system's `readDropRow`.
 */
export function SidebarRowContextMenu({ treeRef, rows, onImport }: SidebarRowContextMenuProps) {
  const menu = useContextMenu<MenuData>()
  const { openAt } = menu
  const navigate = useNavigate()
  // Whether the provider a new chat would actually start under offers a
  // START_HERE terminal (design spec 2.5) — a NARROW, reactive selector
  // (never `.getState()` in render): absence, not a disabled control, so
  // "New thread in Terminal" below is left OUT of `items` entirely rather
  // than pushed disabled. No enabled provider resolved yet is not evidence
  // of "no terminal" — the plain Thread button offers itself unconditionally
  // too and leaves that refusal to `enabledProvider()` at click time; this
  // matches it (providerCanStartOnTerminal(undefined) is permissive).
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
      return { row, locked }
    }
    const onContextMenu = (e: MouseEvent) => {
      if (!(e.target instanceof Element)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const data = resolveMenuData(rowId)
      if (!data) return
      e.preventDefault()
      openAt({ x: e.clientX, y: e.clientY }, data)
    }
    // The row's own "..." button (sidebar-row.tsx, `data-control="repo-menu"`)
    // opens this SAME menu, anchored under the button — explicit user
    // correction: a repo-home row used to carry a second, separate one-item
    // menu of its own (just "Delete Repo"), which drifted out of sync with
    // whatever this menu grew (Import branches, Rename, New folder). One row
    // gets one menu, reachable by right-click OR by the "..." button.
    //
    // Capture phase, not bubble: the button sits inside the row's own
    // `onClick`-to-open div, and every other trailing-cluster button already
    // stops that propagation on the way up (see e.g. the Thread button's own
    // `e.stopPropagation()`). A bubble-phase listener here would race that —
    // whichever runs first wins — where capture always runs first, well
    // before the row (or the button's own bubble handler, if it had one) ever
    // sees the click, so nothing extra is needed on the button itself.
    const onRepoMenuClick = (e: MouseEvent) => {
      if (!(e.target instanceof Element)) return
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

  if (!menu.isOpen || !menu.data) return null
  const { row, locked } = menu.data
  // Rename starts the same inline editor double-click does; the store knows which instance draws it.
  const handlers = {
    navigate,
    rename: (target: SidebarRow) => useSidebarInlineRenameStore.getState().startRenaming(target.id),
    importBranches: (target: SidebarRow) => onImport(target.id),
    fork: (target: SidebarRow) => void handleCreate(target.id, 'workspace', navigate),
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
  // Kill agent sits with the destructive group, ahead of its first separator.
  const destructiveAt = items.findIndex((i) => i.separator)
  items.splice(destructiveAt < 0 ? items.length : destructiveAt, 0, ...killItems)

  return <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
}
