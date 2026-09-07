import { useEffect, useMemo } from 'react'
import { IS_MAC } from '@/utils/platform'
import {
  useRootLayout,
  useParkedViews,
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

/** Module-level so the object identities are stable across renders — inline
 *  literals would hand every view's wrapper a new `style` prop every render. */
const PARKED_VIEW_STYLE: React.CSSProperties = { display: 'none' }
const SHOWING_VIEW_STYLE: React.CSSProperties = {}

export function SplitViewRoot() {
  const rootLayout = useRootLayout()
  const parkedViews = useParkedViews()
  const activeViewId = useActiveViewId()
  const fullscreenPaneId = useFullscreenPaneId()
  const { exitPaneFullscreen } = usePaneActions()

  /**
   * EVERY open view, showing and parked alike, in ONE list at ONE position in
   * the tree, sorted by a key that has nothing to do with which is on screen.
   *
   * That last part is the whole point and was learned the hard way. Rendering
   * the showing view in one place and the parked ones in another looks
   * equivalent — both keep every view mounted — and is not: switching views
   * moves a subtree from one parent to the other, which React can only do by
   * UNMOUNTING it and mounting a new one. Measured live: parking a view with a
   * running shell in it destroyed the xterm, and because re-initialisation is
   * gated on `isVisible` (terminal.tsx), the terminal never came back — a
   * parked view quietly lost the thing parking it was supposed to protect.
   *
   * One list, `key`ed by view id, so a view keeps its identity and its DOM no
   * matter which is active: switching only flips `showing` and a `style`, and
   * React reorders (never remounts) if the sort puts it elsewhere.
   */
  const views = useMemo(() => {
    const all = [
      { id: activeViewId, layout: rootLayout, showing: true },
      ...Object.entries(parkedViews).map(([id, layout]) => ({ id, layout, showing: false })),
    ]
    // Sorted by id, NOT by "active first" — an order that moved with the
    // active view would reshuffle the list on every switch for no reason.
    return all.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  }, [activeViewId, rootLayout, parkedViews])

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
