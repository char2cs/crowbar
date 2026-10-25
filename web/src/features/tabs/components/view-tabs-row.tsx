import { memo, useRef } from 'react'
import { useStore } from 'zustand'
import { useNavigate } from '@tanstack/react-router'
import { useSidebar } from '@/components/ui/sidebar'
import { RecentsViewRow } from '@/components/sidebar/view-row'
import { focusRecent, closeRecent } from '@/components/sidebar/lib/recents-actions'
import { performSidebarDrop, performSidebarPaneDrop } from '@/components/sidebar/lib/drop-actions'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { BuildReadout } from '@/components/layout/build-readout'
import { useProjectViewIds } from '@/features/panes/hooks/use-project-view-ids'
import { viewMembers } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { PaneTopRow } from './pane-top-row'
import TabNavigationButtons from './tab-navigation-buttons'

// Every tab is this wide, a split included.
const ROW_WIDTH_PX = 176

/**
 * The sidebar's Recents rows laid out across the top: one per view of the
 * active project, switched, closed and dragged exactly as in the sidebar.
 * Draws nothing until a view exists. Owns the sidebar-reopen toggle while it
 * is mounted, since the panes below it are no longer at the window's top edge.
 */
export const ViewTabsRow = memo(function ViewTabsRow() {
  const navigate = useNavigate()
  const viewIds = useProjectViewIds()
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const { rowRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge } = usePaneTopRowEdges([sidebarPosition])
  const scrollRef = useRef<HTMLDivElement>(null)
  const rowsRef = useRef(new Map<string, { row: SidebarRowType; viewId: string }>())
  const registerRow = (row: SidebarRowType, viewId: string) => {
    rowsRef.current.set(row.id, { row, viewId })
  }
  const drag = useSidebarDrag({
    scrollRef,
    // A tab moves as a whole: grabbing any chat of a split carries them all.
    subjectsFor: (rowId) => {
      const viewId = rowsRef.current.get(rowId)?.viewId
      if (!viewId) return []
      const rows: SidebarRowType[] = []
      for (const entry of rowsRef.current.values()) {
        if (entry.viewId === viewId) rows.push(entry.row)
      }
      return rows
    },
    onDrop: performSidebarDrop,
    onPaneDrop: performSidebarPaneDrop,
  })

  if (viewIds.length === 0) return null

  const toggle = (
    <div className="flex h-8 shrink-0 items-center">
      <TabNavigationButtons
        isBottomPane={false}
        sidebarOpen={sidebarOpen}
        sidebarPosition={sidebarPosition}
        onToggleSidebar={toggleSidebar}
      />
    </div>
  )
  return (
    <PaneTopRow
      rowRef={rowRef}
      isBottomPane={false}
      isAtLeftEdge={isAtLeftEdge}
      isAtTopEdge={isAtTopEdge}
      className="bg-transparent py-0 pr-2"
    >
      {!sidebarOpen && sidebarPosition === 'left' && isAtLeftEdge && toggle}
      <div
        ref={scrollRef}
        data-testid="view-tabs"
        data-drop-axis="x"
        data-tauri-drag-region
        className="tab-scrollbar flex min-w-0 flex-1 items-center overflow-x-auto overflow-y-hidden"
      >
        {viewIds.map((viewId) => (
          <ViewTabSlot key={viewId} viewId={viewId}>
            <RecentsViewRow
              viewId={viewId}
              onFocus={(id) => focusRecent(id, useSidebarStore.getState().repos, navigate)}
              onClose={closeRecent}
              drag={drag}
              registerRow={registerRow}
            />
          </ViewTabSlot>
        ))}
      </div>
      {/* A zero-width anchor in flow: the readout hangs off its right edge, so it sits left of the reopen toggle that follows. */}
      {!sidebarOpen && (
        <div className="relative w-0 shrink-0 self-stretch">
          <BuildReadout className="right-2" />
        </div>
      )}
      {!sidebarOpen && sidebarPosition === 'right' && isAtRightEdge && toggle}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </PaneTopRow>
  )
})

function ViewTabSlot({ viewId, children }: { viewId: string; children: React.ReactNode }) {
  const hasMembers = useStore(windowPaneStore, (s) => viewMembers(s, viewId).length > 0)
  if (!hasMembers) return null
  return (
    <div className="shrink-0" style={{ width: ROW_WIDTH_PX }}>
      {children}
    </div>
  )
}
