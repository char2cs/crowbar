import { cn } from '@/lib/utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  ROW_BASE,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
} from '@/components/layout/workspace-row-base'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { AffordanceRow } from '@/components/sidebar/affordance-row'
import { useSidebarDrag, type SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

interface SidebarTreeProps {
  /** One project's rows, flat, parentId-linked. */
  rows: SidebarRowType[]
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

const byOrder = (a: SidebarRowType, b: SidebarRowType) => a.order - b.order

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
  roots.sort(byOrder)
  for (const siblings of childrenByParent.values()) siblings.sort(byOrder)

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
        <SidebarRow
          row={row}
          depth={depth}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          onToggleFold={(id) => useSidebarStore.getState().toggleChatRow(id)}
          folded={folded}
          dragProps={drag.dragProps(row, { path: childPath, expanded: !folded, hasChildren })}
          isDragging={drag.draggingIds.has(row.id)}
          isNestTarget={drag.nestTargetId === row.id}
          onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        />
        {!folded &&
          (hasChildren ? (
            children!.map((child) => renderRow(child, depth + 1, childPath))
          ) : (
            <div
              className={ROW_INDENT_TRANSITION}
              style={{ marginInlineStart: (depth + 1) * ROW_INDENT_STEP }}
            >
              <div
                className={cn(ROW_BASE, ROW_INACTIVE, 'group cursor-default justify-end pr-2.5')}
              >
                <AffordanceRow
                  onCreateThread={() => onCreate(row.id, 'thread')}
                  onCreateWorkspace={
                    row.ownsWorktree ? () => onCreate(row.id, 'workspace') : undefined
                  }
                />
              </div>
            </div>
          ))}
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
