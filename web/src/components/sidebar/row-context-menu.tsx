import { useEffect, type RefObject } from 'react'
import { DownloadSimple, Folder, Lock, LockOpen, PencilSimpleLine } from '@phosphor-icons/react'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { useSidebarStore } from '@/lib/store/sidebar'
import { performCreateFolder, performSetWorkspaceLock } from '@/components/sidebar/lib/row-actions'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

interface SidebarRowContextMenuProps {
  treeRef: RefObject<HTMLElement | null>
  /** Every visible row, to look up kind/ownsWorktree/parentId by id. */
  rows: SidebarRow[]
  /** Opens rename-dialog.tsx for the row. */
  onRename: (rowId: string) => void
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
 * The sidebar's right-click menu — rename, lock/unlock, branch import, and
 * "New folder", the four verbs Task 8's unification left with no home on
 * `SidebarRow`'s four-prop surface.
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
export function SidebarRowContextMenu({
  treeRef,
  rows,
  onRename,
  onImport,
}: SidebarRowContextMenuProps) {
  const menu = useContextMenu<MenuData>()
  const { openAt } = menu

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onContextMenu = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const row = rows.find((r) => r.id === rowId)
      if (!row) return
      const locked = useSidebarStore
        .getState()
        .repos.some((repo) => repo.workspaces.some((w) => w.id === rowId && w.status === 'locked'))
      e.preventDefault()
      openAt({ x: e.clientX, y: e.clientY }, { row, locked })
    }
    tree.addEventListener('contextmenu', onContextMenu)
    return () => tree.removeEventListener('contextmenu', onContextMenu)
  }, [treeRef, rows, openAt])

  if (!menu.isOpen || !menu.data) return null
  const { row, locked } = menu.data
  const isProjectHome = row.kind === 'branch' && row.parentId === null

  const items: ContextMenuItem[] = [
    {
      id: 'rename',
      label: 'Rename',
      icon: <PencilSimpleLine />,
      onClick: () => onRename(row.id),
    },
  ]

  if (row.ownsWorktree) {
    items.push(
      locked
        ? {
            id: 'unlock',
            label: 'Unlock',
            icon: <LockOpen />,
            onClick: () => void performSetWorkspaceLock(row.id, false),
          }
        : {
            id: 'lock',
            label: 'Lock',
            icon: <Lock />,
            onClick: () => void performSetWorkspaceLock(row.id, true),
          },
    )
  }

  if (isProjectHome) {
    items.push({
      id: 'import',
      label: 'Import branches',
      icon: <DownloadSimple />,
      onClick: () => onImport(row.id),
    })
  }

  if (row.kind === 'branch' || row.kind === 'folder') {
    items.push({
      id: 'new-folder',
      label: 'New folder',
      icon: <Folder />,
      onClick: () => void performCreateFolder(row.id),
    })
  }

  return <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
}
