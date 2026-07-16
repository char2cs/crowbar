import { memo, useCallback, useRef } from 'react'
import { usePaneActions, usePaneById } from '@/features/workspace/stores/hooks/use-pane-store'
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
  // Subscribe to ONLY this node's own pane — never the whole `panes` record.
  // immer's structural sharing keeps every sibling pane's reference identical
  // across a mutation, so a change to pane A (tab switch, focus, open/close)
  // leaves pane B's selector output === its previous value and B's leaf
  // renderer is skipped. This is what stops a pane-local change from
  // re-rendering the entire layout tree at every recursion level. Split nodes
  // pass their own (non-pane) id, which resolves to null here and is unused.
  const pane = usePaneById(node.id)
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
