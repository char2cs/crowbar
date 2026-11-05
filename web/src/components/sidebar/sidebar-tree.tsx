import { useStore } from 'zustand'
import { useChatWorking } from '@/features/agent/hooks/use-chat-entity'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarPaneZone } from '@/components/sidebar/lib/sidebar-drag-hover'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectChatHasView } from '@/features/panes/lib/view-selectors'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { sidebarRowTree } from '@/components/sidebar/lib/row-tree'

interface SidebarTreeProps {
  /** One project's rows, flat, parentId-linked. */
  rows: readonly SidebarRowType[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  /** The panel's scroll container, which an edge-held drag scrolls; shared with the Recents
   *  row, since both sit in one scroll region per space. */
  scrollRef: React.RefObject<HTMLElement | null>
  onDrop: (subjects: SidebarRowType[], target: SidebarRowType, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRowType[], paneId: string, zone: SidebarPaneZone) => void
}

// The component renderRow defers to, so hooks run per row: it subscribes one row to the two live
// facts deriveRows seeds inert, pane membership and the chat's server-folded `working`.
export function SidebarTreeRow({
  row,
  ...rest
}: { row: SidebarRowType } & Omit<React.ComponentProps<typeof SidebarRow>, 'row'>) {
  const hasView = useStore(windowPaneStore, (s) => selectChatHasView(s, row.id))
  // A row's own id is its chat's; a folder names none, so it reads false.
  const chatWorking = useChatWorking(row.kind === 'folder' ? '' : row.id)
  // `row.working` is any chat of the worktree mid-turn, which answers for an
  // unopened workspace too. Operation work spins via selectEntityBusy instead.
  const working = chatWorking || row.working
  // This IS the tree's render path: a row with a view reads as open, never grey.
  return <SidebarRow row={{ ...row, hasView, working }} {...rest} hasViewIdle />
}

// Walks flat, parentId-linked rows into the nested tree (spec §4). A row with children gets
// the fold chevron; fold state lives in the tree UI store, which survives remounts.
export function SidebarTree({
  rows,
  onOpen,
  onTrash,
  onCreate,
  scrollRef,
  onDrop,
  onPaneDrop,
}: SidebarTreeProps) {
  const collapsed = useTreeUi((s) => s.collapsed)

  const { roots, childrenByParent } = sidebarRowTree(rows)

  // Single-row drags only: the unified tree has no multiselect yet.
  const drag = useSidebarDrag({
    scrollRef,
    subjectsFor: (rowId) => {
      const row = rows.find((r) => r.id === rowId)
      return row ? [row] : []
    },
    onDrop,
    onPaneDrop,
  })

  function renderRow(row: SidebarRowType, depth: number, path: string) {
    const folded = collapsed.has(row.id)
    const children = childrenByParent.get(row.id)
    const hasChildren = !!children && children.length > 0
    const childPath = `${path}${row.id}/`

    return (
      <div key={row.id}>
        <SidebarTreeRow
          row={row}
          depth={depth}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          // A childless row draws nothing below it, so it gets no fold chevron.
          onToggleFold={hasChildren ? (id) => treeUiStore.getState().toggleChatRow(id) : undefined}
          folded={folded}
          dragProps={drag.dragProps(row, { path: childPath, expanded: !folded, hasChildren })}
          isDragging={drag.draggingIds.has(row.id)}
          isNestTarget={drag.nestTargetId === row.id}
          onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        />
        {!folded && hasChildren && children!.map((child) => renderRow(child, depth + 1, childPath))}
      </div>
    )
  }

  return (
    <>
      {roots.map((row) => renderRow(row, 0, '/'))}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </>
  )
}
