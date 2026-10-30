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
import { recentsRowOf, registerRecentsRow } from '@/components/sidebar/lib/recents-row-registry'
import { BuildReadout } from '@/components/layout/build-readout'
import { useProjectViewIds } from '@/features/panes/hooks/use-project-view-ids'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds, viewMembers } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { PaneTopRow } from './pane-top-row'
import TabNavigationButtons from './tab-navigation-buttons'

// Tabs share the strip equally between these bounds, a split included; the
// strip scrolls only once every tab is at the minimum.
const TAB_MAX_WIDTH_PX = 240
const TAB_MIN_WIDTH_PX = 112

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
  const drag = useSidebarDrag({
    scrollRef,
    strip: () => scrollRef.current,
    // A tab moves as a whole: grabbing any chat of a split carries them all.
    // Membership is read from the pane store at grab time, so a chat closed
    // out of the split is not carried back; the registry only supplies row data.
    subjectsFor: (rowId) => {
      const { panes, views } = windowPaneStore.getState()
      const viewId = panes[chatPaneIndex(panes).get(rowId) ?? '']?.viewId
      if (!viewId) return []
      return viewChatIds({ panes, views }, viewId).flatMap((id) => recentsRowOf(id) ?? [])
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
      className="chrome-surface bg-transparent py-0 pr-2"
    >
      {!sidebarOpen && sidebarPosition === 'left' && isAtLeftEdge && toggle}
      <div
        ref={scrollRef}
        data-testid="view-tabs"
        data-view-tab-strip
        data-drop-axis="x"
        data-tauri-drag-region
        // py-1 / -my-1: room for the lifted tab's outline and shadow inside the clip, with no layout change.
        className="tab-scrollbar -my-1 flex min-w-0 flex-1 items-center overflow-x-auto overflow-y-hidden py-1"
      >
        {viewIds.map((viewId) => (
          <ViewTabSlot key={viewId} viewId={viewId}>
            <RecentsViewRow
              viewId={viewId}
              onFocus={(id) => focusRecent(id, useSidebarStore.getState().repos, navigate)}
              onClose={closeRecent}
              drag={drag}
              registerRow={registerRecentsRow}
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
  const pinned = useStore(windowPaneStore, (s) => !!s.views[viewId]?.pinned)
  if (!hasMembers) return null
  return (
    <div
      data-pinned={pinned || undefined}
      style={{
        flex: `1 1 ${TAB_MAX_WIDTH_PX}px`,
        minWidth: TAB_MIN_WIDTH_PX,
        maxWidth: TAB_MAX_WIDTH_PX,
      }}
    >
      {children}
    </div>
  )
}
