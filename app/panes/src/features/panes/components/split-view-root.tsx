import { useEffect, useMemo } from 'react'
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
import { useMountedViews } from '../hooks/use-mounted-views'
import { useHasViewStrip } from '../hooks/use-tab-view-ids'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

const PARKED_VIEW_STYLE: React.CSSProperties = { display: 'none' }
const SHOWING_VIEW_STYLE: React.CSSProperties = {}

export function SplitViewRoot() {
  const records = useViews()
  const stage = useStageLayout()
  const activeViewId = useActiveViewId()
  const fullscreenPaneId = useFullscreenPaneId()
  const { exitPaneFullscreen } = usePaneActions()

  const views = useMemo(() => {
    const all = Object.values(records).map((view) => ({
      id: view.id,
      layout: view.layout,
      showing: view.id === activeViewId,
    }))
    all.push({ id: getFirstLeafId(stage), layout: stage, showing: activeViewId === null })
    return all.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  }, [records, stage, activeViewId])

  const mountedViews = useMountedViews(views)

  const fullscreenPane = usePaneById(fullscreenPaneId ?? '')

  useEffect(() => {
    if (fullscreenPaneId && !fullscreenPane) exitPaneFullscreen()
  }, [exitPaneFullscreen, fullscreenPane, fullscreenPaneId])

  const isBottomPaneVisible = useUIState((state) => state.isBottomPaneVisible)
  const hasViewStrip = useHasViewStrip()
  const rootPosition = useMemo(
    () => ({ ...ROOT_PANE_POSITION, atTop: !hasViewStrip, atBottom: !isBottomPaneVisible }),
    [hasViewStrip, isBottomPaneVisible],
  )

  const titleBarHeight = CHROME_ROW_HEIGHT_PX
  const footerHeight = 32

  return (
    <>
      {mountedViews.map((view) => (
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
