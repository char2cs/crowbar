import { useStore } from 'zustand'
import { useChatWorking } from '@/features/agent/hooks/use-chat-entity'
import { useSidebarStore } from '@/lib/store/sidebar'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useSidebarDrag, type SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectChatHasView } from '@/features/panes/lib/view-selectors'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'

interface SidebarTreeProps {
  /** One project's rows, flat, parentId-linked. */
  rows: readonly SidebarRowType[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  /** The panel's own scroll container — what an edge-held drag scrolls
   *  (Task 21). Shared with `RecentsBand`, since the two sit in one scroll
   *  region per space. */
  scrollRef: React.RefObject<HTMLElement | null>
  onDrop: (subjects: SidebarRowType[], target: SidebarRowType, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRowType[], paneId: string, zone: SidebarPaneZone) => void
}

/**
 * The component `renderRow` defers to, so hooks run per row: it subscribes one
 * row to the two live facts `deriveRows` seeds inert, pane membership and
 * the chat's server-folded `working`.
 */
function SidebarTreeRow({
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

/**
 * Walks a flat, parentId-linked `SidebarRow[]` into the real nested tree —
 * spec §4: one project's rows render as siblings, with no rule between them
 * (the horizontal separators that used to divide projects are gone now that a
 * space holds exactly one).
 *
 * Every row is a container (spec §3.1: "a container can always be given
 * something"), so every row gets the fold chevron. A container currently
 * holding nothing renders the one affordance row (§3.5) in place of its
 * children, rather than nothing.
 *
 * Fold state reuses `collapsedChatRows` off the always-alive `useSidebarStore`
 * — the same set `agent-chats-panel.tsx` folds Chats-panel rows into today —
 * rather than a second fold-state store. It already survives what this tree
 * will too: a remount local component state would not.
 */
export function SidebarTree({
  rows,
  onOpen,
  onTrash,
  onCreate,
  scrollRef,
  onDrop,
  onPaneDrop,
}: SidebarTreeProps) {
  const collapsed = useSidebarStore((s) => s.collapsedChatRows)

  const ids = new Set(rows.map((r) => r.id))
  const childrenByParent = new Map<string, SidebarRowType[]>()
  const roots: SidebarRowType[] = []
  for (const row of rows) {
    // A row whose parent isn't in this project's own set is a root too —
    // defensive against a dangling edge rather than silently dropping it.
    const parentId = row.parentId !== null && ids.has(row.parentId) ? row.parentId : null
    if (parentId === null) {
      roots.push(row)
      continue
    }
    const siblings = childrenByParent.get(parentId)
    if (siblings) siblings.push(row)
    else childrenByParent.set(parentId, [row])
  }
  roots.sort(compareSidebarRows)
  for (const siblings of childrenByParent.values()) siblings.sort(compareSidebarRows)

  // Single-row drags only — the new unified tree has no multiselect yet
  // (Task 8 left `sidebar-selection.ts` orphaned), so a drag always carries
  // exactly the grabbed row.
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
          // Childless only ever renders nothing below itself regardless of
          // fold state (see the comment just below — the ghost ISN'T there
          // to expand into any more), so the chevron offered a toggle with
          // no visible effect — explicit product correction from the old
          // "every row is a container" stance (spec §3.1) this used to
          // follow unconditionally.
          onToggleFold={
            hasChildren ? (id) => useSidebarStore.getState().toggleChatRow(id) : undefined
          }
          folded={folded}
          dragProps={drag.dragProps(row, { path: childPath, expanded: !folded, hasChildren })}
          isDragging={drag.draggingIds.has(row.id)}
          isNestTarget={drag.nestTargetId === row.id}
          onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        />
        {/* A childless folder used to render a second, unlabeled "ghost" row
            here just to hold Fork/Thread buttons — removed. A folder's own
            Fork button now lives on its own row (sidebar-row.tsx), the same
            place every other kind's already did; there is nothing left for
            an empty folder to draw underneath itself. */}
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
