import { memo, useCallback, useRef } from 'react'
import { usePaneActions, usePanes } from '@/features/workspace/stores/hooks/use-pane-store'
import type { LayoutNode, PanePosition } from '../types/pane'
import { ROOT_PANE_POSITION } from '../types/pane'
import { PaneContainer } from './pane-container'
import { PaneBoundary } from './pane-boundary'
import { PaneSash } from './pane-sash'

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

  const containerRef = useRef<HTMLDivElement>(null)
  const firstPaneRef = useRef<HTMLDivElement>(null)
  const secondPaneRef = useRef<HTMLDivElement>(null)

  const splitId = node.type === 'split' ? node.id : null
  const handleResizeCommit = useCallback(
    (sizes: [number, number]) => {
      if (splitId) resizePaneSplit(splitId, 0, sizes)
    },
    [splitId, resizePaneSplit],
  )

  if (node.type === 'pane') {
    if (hiddenPaneId === node.id) {
      return <div className="h-full w-full bg-transparent" aria-hidden="true" />
    }
    const pane = panes[node.id]
    if (!pane) return null
    return (
      <PaneBoundary paneId={node.id}>
        <PaneContainer pane={pane} position={position} />
      </PaneBoundary>
    )
  }

  const isHorizontal = node.direction === 'horizontal'
  const firstPos = binaryPosition(position, true, node.direction)
  const secondPos = binaryPosition(position, false, node.direction)

  return (
    <div
      ref={containerRef}
      className={`flex h-full w-full ${isHorizontal ? 'flex-row' : 'flex-col'}`}
    >
      <div
        ref={firstPaneRef}
        className="min-h-0 min-w-0 grow-0 shrink"
        style={{ flexBasis: `${node.sizes[0]}%` }}
      >
        <PaneNodeRenderer node={node.first} hiddenPaneId={hiddenPaneId} position={firstPos} />
      </div>
      <PaneSash
        direction={node.direction}
        sizes={node.sizes}
        containerRef={containerRef}
        firstPaneRef={firstPaneRef}
        secondPaneRef={secondPaneRef}
        onResizeCommit={handleResizeCommit}
      />
      <div
        ref={secondPaneRef}
        className="min-h-0 min-w-0 grow-0 shrink"
        style={{ flexBasis: `${node.sizes[1]}%` }}
      >
        <PaneNodeRenderer node={node.second} hiddenPaneId={hiddenPaneId} position={secondPos} />
      </div>
    </div>
  )
})
