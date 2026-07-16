import { useEffect, useMemo } from 'react'
import { IS_MAC } from '@/utils/platform'
import {
  useRootLayout,
  useFullscreenPaneId,
  usePaneActions,
  usePaneById,
} from '@/features/workspace/stores/hooks/use-pane-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { PaneContainer } from './pane-container'
import { PaneNodeRenderer } from './pane-node-renderer'
import { PaneBoundary } from './pane-boundary'
import { ROOT_PANE_POSITION } from '../types/pane'

export function SplitViewRoot() {
  const rootLayout = useRootLayout()
  const fullscreenPaneId = useFullscreenPaneId()
  const { exitPaneFullscreen } = usePaneActions()

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
      <div className="h-full w-full">
        <PaneNodeRenderer
          node={rootLayout}
          hiddenPaneId={fullscreenPaneId}
          position={rootPosition}
        />
      </div>
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
