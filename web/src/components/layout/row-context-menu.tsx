import { useEffect, type RefObject } from 'react'
import { FolderPlus, Trash2 } from 'lucide-react'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { dragSubjectsFor, readDropRow, readSelectedSubjects } from './drop-target-dom'
import { rowMenuFor } from './row-menu-model'
import { useWorkspaceTreeActions } from './workspace-tree-context'
import type { DragSubject } from './drop-rules'

/**
 * The sidebar's right-click menu.
 *
 * It is where the bulk actions live, because the selection action bar they
 * would otherwise need was claiming the foot of the sidebar — which is where
 * held rows wait now — to offer things that already had a home.
 *
 * What it acts on comes from `dragSubjectsFor()`, the same function a drag
 * uses: right-clicking a row inside the multiselection acts on the whole
 * selection, right-clicking one outside it acts on that row and leaves the
 * selection alone. Two implementations of that rule would eventually disagree,
 * and the drag's is the one that is already tested.
 *
 * A SIBLING of the tree rather than a hook inside it, and listening natively
 * rather than through an `onContextMenu` prop. Both for the same reason: with
 * the open/closed state inside the tree component, opening this popup
 * re-rendered every row in the sidebar (measured: eight of eight) to draw a
 * menu that is not part of the tree at all. Out here it re-renders itself.
 */
export function RowContextMenu({ treeRef }: { treeRef: RefObject<HTMLElement | null> }) {
  const menu = useContextMenu<DragSubject[]>()
  const { removeRows, groupIntoFolder } = useWorkspaceTreeActions()
  const { openAt } = menu

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onContextMenu = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const row = el ? readDropRow(el) : null
      if (!row) return
      const subjects = dragSubjectsFor(row, readSelectedSubjects())
      // Nothing on offer means no popup: an empty menu, or one of nothing but
      // disabled items, is a worse answer than the row simply not having one.
      if (rowMenuFor(subjects).length === 0) return
      e.preventDefault()
      openAt({ x: e.clientX, y: e.clientY }, subjects)
    }
    tree.addEventListener('contextmenu', onContextMenu)
    return () => tree.removeEventListener('contextmenu', onContextMenu)
  }, [treeRef, openAt])

  if (!menu.isOpen) return null

  const subjects = menu.data ?? []
  const items: ContextMenuItem[] = rowMenuFor(subjects).map((entry) =>
    entry.id === 'group'
      ? {
          id: entry.id,
          label: entry.label,
          icon: <FolderPlus />,
          onClick: () => groupIntoFolder(subjects),
        }
      : {
          id: entry.id,
          label: entry.label,
          icon: <Trash2 />,
          className: 'text-destructive',
          onClick: () => removeRows(subjects),
        },
  )

  return <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
}
