import { memo, useCallback, useRef } from 'react'
import type { PanelSize } from 'react-resizable-panels'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { usePaneActions, usePanes } from '@/features/workspace/stores/hooks/use-pane-store'
import type { LayoutNode, PanePosition } from '../types/pane'
import { ROOT_PANE_POSITION } from '../types/pane'
import { MIN_PANE_SIZE } from '../constants/pane'
import { PaneContainer } from './pane-container'
import { PaneBoundary } from './pane-boundary'

interface PaneNodeRendererProps {
  node: LayoutNode
  hiddenPaneId?: string | null
  position?: PanePosition
}

function binaryPosition(
  parent: PanePosition,
  isFirst: boolean,
  direction: 'horizontal' | 'vertical',
): PanePosition {
  if (direction === 'horizontal') {
    return {
      atLeft: isFirst && parent.atLeft,
      atTop: parent.atTop,
      atRight: !isFirst && parent.atRight,
      atBottom: parent.atBottom,
    }
  }
  return {
    atLeft: parent.atLeft,
    atTop: isFirst && parent.atTop,
    atRight: parent.atRight,
    atBottom: !isFirst && parent.atBottom,
  }
}

export const PaneNodeRenderer = memo(function PaneNodeRenderer({
  node,
  hiddenPaneId = null,
  position = ROOT_PANE_POSITION,
}: PaneNodeRendererProps) {
  const panes = usePanes()
  const { resizePaneSplit } = usePaneActions()
  const firstSizeRef = useRef(node.type === 'split' ? node.sizes[0] : 50)
  const secondSizeRef = useRef(node.type === 'split' ? node.sizes[1] : 50)
  const isDraggingRef = useRef(false)

  const handleFirstResize = useCallback((size: PanelSize) => {
    firstSizeRef.current = size.asPercentage
  }, [])

  const handleSecondResize = useCallback((size: PanelSize) => {
    secondSizeRef.current = size.asPercentage
  }, [])

  const handleLayoutChange = useCallback(() => {
    if (!isDraggingRef.current) {
      isDraggingRef.current = true
      document.documentElement.setAttribute('data-pane-resizing', '1')
    }
  }, [])

  const handleLayoutChanged = useCallback(() => {
    isDraggingRef.current = false
    document.documentElement.removeAttribute('data-pane-resizing')
    window.dispatchEvent(new CustomEvent('pane-resize-end'))
    if (node.type === 'split') {
      resizePaneSplit(node.id, 0, [firstSizeRef.current, secondSizeRef.current])
    }
  }, [node, resizePaneSplit])

  if (node.type === 'pane') {
    if (hiddenPaneId === node.id) {
      return <div className="h-full w-full bg-background" aria-hidden="true" />
    }
    const pane = panes[node.id]
    if (!pane) return null
    return (
      <PaneBoundary paneId={node.id}>
        <PaneContainer pane={pane} position={position} />
      </PaneBoundary>
    )
  }

  const firstPos = binaryPosition(position, true, node.direction)
  const secondPos = binaryPosition(position, false, node.direction)
  const minPct = `${MIN_PANE_SIZE}%`

  return (
    <ResizablePanelGroup
      orientation={node.direction}
      onLayoutChange={handleLayoutChange}
      onLayoutChanged={handleLayoutChanged}
      className="h-full w-full"
    >
      <ResizablePanel
        defaultSize={`${node.sizes[0]}%`}
        minSize={minPct}
        onResize={handleFirstResize}
      >
        <div className="h-full w-full overflow-hidden">
          <PaneNodeRenderer node={node.first} hiddenPaneId={hiddenPaneId} position={firstPos} />
        </div>
      </ResizablePanel>
      <ResizableHandle />
      <ResizablePanel
        defaultSize={`${node.sizes[1]}%`}
        minSize={minPct}
        onResize={handleSecondResize}
      >
        <div className="h-full w-full overflow-hidden">
          <PaneNodeRenderer node={node.second} hiddenPaneId={hiddenPaneId} position={secondPos} />
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  )
})
