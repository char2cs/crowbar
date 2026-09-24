import { useEffect, useMemo } from 'react'
import { IS_MAC } from '@/utils/platform'
import {
  useViews,
  useStageLayout,
  useActiveViewId,
  useFullscreenPaneId,
  usePaneActions,
  usePaneById,
} from '@/features/workspace/stores/hooks/use-pane-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { PaneContainer } from './pane-container'
import { PaneNodeRenderer } from './pane-node-renderer'
import { PaneBoundary } from './pane-boundary'
import { ROOT_PANE_POSITION } from '../types/pane'
import { getFirstLeafId } from '../utils/pane-layout'

/** Module-level so the object identities are stable across renders — inline
 *  literals would hand every view's wrapper a new `style` prop every render. */
const PARKED_VIEW_STYLE: React.CSSProperties = { display: 'none' }
const SHOWING_VIEW_STYLE: React.CSSProperties = {}

export function SplitViewRoot() {
  const records = useViews()
  const stage = useStageLayout()
  const activeViewId = useActiveViewId()
  const fullscreenPaneId = useFullscreenPaneId()
  const { exitPaneFullscreen } = usePaneActions()

  /**
   * Every record plus the stage, in ONE list keyed by id and sorted by
   * something unrelated to which is showing: rendering the showing view in a
   * different place than the others would REMOUNT it on every switch, which
   * destroys a parked terminal's xterm for good. The stage is keyed by its
   * first pane id — the id a promoted stage's record takes — so a chat landing
   * in it keeps its DOM.
   */
  const views = useMemo(() => {
    const all = Object.values(records).map((view) => ({
      id: view.id,
      layout: view.layout,
      showing: view.id === activeViewId,
    }))
    all.push({ id: getFirstLeafId(stage), layout: stage, showing: activeViewId === null })
    return all.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  }, [records, stage, activeViewId])

  // Subscribe to ONLY the fullscreen pane (or nothing) — never the whole
  // `panes` record. Reading the whole record re-rendered SplitViewRoot, and
  // therefore the entire layout tree beneath it, on every pane mutation. This
  // id-scoped selector is referentially stable across unrelated pane changes.
  const fullscreenPane = usePaneById(fullscreenPaneId ?? '')

  useEffect(() => {
    if (fullscreenPaneId && !fullscreenPane) exitPaneFullscreen()
  }, [exitPaneFullscreen, fullscreenPane, fullscreenPaneId])

  const isBottomPaneVisible = useUIState((state) => state.isBottomPaneVisible)
  const rootPosition = useMemo(
    () => ({ ...ROOT_PANE_POSITION, atBottom: !isBottomPaneVisible }),
    [isBottomPaneVisible],
  )

  const titleBarHeight = IS_MAC ? 44 : 28
  const footerHeight = 32

  return (
    <>
      {/* ONLY THE SHOWING VIEW OCCUPIES THE CONTENT AREA. The others take no
          space and paint nothing, so "the active view fills the screen and
          whatever was there goes away from view" is a property of the layout
          itself, not of a visibility check inside each pane.

          A parked view is KEPT MOUNTED and hidden with `display: none` — the
          same dormancy `WorkspaceHost` already runs every retained workspace
          under (workspace-slot-style.ts, where `content-visibility` is
          recorded as tried and reverted for melting the CPU). Unmounting
          would be cheaper on paper and is wrong in practice: a pane's shell
          terminal disposes its xterm on unmount and remounts onto a live
          transport that never re-attaches — blank, scrollback gone — and its
          Monaco models are refcount-released with their undo history.
          Switching views is a glance, not a close; it may not cost the state
          of what you glanced away from. `closePane` / `closeView` remain the
          only things that end a view, and they still run the full teardown.

          `inert` so nothing off screen is focusable, clickable or reachable by
          keyboard. */}
      {views.map((view) => (
        <div
          key={view.id}
          className="h-full w-full"
          data-view-root={view.id}
          data-parked-view={view.showing ? undefined : view.id}
          style={view.showing ? SHOWING_VIEW_STYLE : PARKED_VIEW_STYLE}
          inert={!view.showing}
        >
          <PaneNodeRenderer
            node={view.layout}
            hiddenPaneId={view.showing ? fullscreenPaneId : null}
            position={rootPosition}
            showing={view.showing}
          />
        </div>
      ))}
      {fullscreenPane && (
        <div
          className="fixed inset-x-2 z-[10040]"
          style={{ top: `${titleBarHeight + 8}px`, bottom: `${footerHeight + 8}px` }}
        >
          <div className="h-full overflow-hidden rounded-xl border border-border/80 bg-transparent shadow-2xl">
            <PaneBoundary paneId={fullscreenPane.id}>
              <PaneContainer pane={fullscreenPane} />
            </PaneBoundary>
          </div>
        </div>
      )}
    </>
  )
}
