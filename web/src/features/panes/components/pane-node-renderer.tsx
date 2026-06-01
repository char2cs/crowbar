import { memo, useCallback, useMemo, useRef } from 'react'
import { usePaneActions, usePanes } from '@/features/workspace/stores/hooks/use-pane-store'
import type { LayoutNode, PanePosition } from '../types/pane'
import { ROOT_PANE_POSITION } from '../types/pane'
import { flattenForRender, type FlatLayoutEntry } from '../utils/pane-layout'
import { PaneContainer } from './pane-container'
import { PaneResizeHandle } from './pane-resize-handle'
import { PaneBoundary } from './pane-boundary'

interface PaneNodeRendererProps {
  node: LayoutNode
  hiddenPaneId?: string | null
  position?: PanePosition
}

interface FlatResizeHandleProps {
  direction: 'horizontal' | 'vertical'
  index: number
  entries: FlatLayoutEntry[]
  splitContainerRef: React.RefObject<HTMLElement | null>
  onReset: () => void
  onResize: (index: number, sizes: [number, number]) => void
}

const FlatResizeHandle = memo(function FlatResizeHandle({
  direction,
  index,
  entries,
  splitContainerRef,
  onReset,
  onResize,
}: FlatResizeHandleProps) {
  const handleResize = useCallback(
    (sizes: [number, number]) => onResize(index, sizes),
    [index, onResize],
  )
  return (
    <PaneResizeHandle
      direction={direction}
      index={index}
      initialSizes={[entries[index].size, entries[index + 1].size]}
      splitContainerRef={splitContainerRef}
      onResize={handleResize}
      onReset={onReset}
    />
  )
})

function childPosition(
  parent: PanePosition,
  index: number,
  total: number,
  direction: 'horizontal' | 'vertical',
): PanePosition {
  const isFirst = index === 0
  const isLast = index === total - 1
  if (direction === 'horizontal') {
    return {
      atLeft: isFirst && parent.atLeft,
      atTop: parent.atTop,
      atRight: isLast && parent.atRight,
      atBottom: parent.atBottom,
    }
  }
  return {
    atLeft: parent.atLeft,
    atTop: isFirst && parent.atTop,
    atRight: parent.atRight,
    atBottom: isLast && parent.atBottom,
  }
}

export function PaneNodeRenderer({
  node,
  hiddenPaneId = null,
  position = ROOT_PANE_POSITION,
}: PaneNodeRendererProps) {
  const panes = usePanes()
  const { distributePaneSplit, resizePaneSplit } = usePaneActions()
  const splitContainerRef = useRef<HTMLElement | null>(null)

  const flatEntries = useMemo(() => {
    if (node.type !== 'split') return null
    return flattenForRender(node)
  }, [node])

  const handleFlatResize = useCallback(
    (index: number, sizes: [number, number]) => {
      if (node.type !== 'split') return
      resizePaneSplit(node.id, index, sizes)
    },
    [node, resizePaneSplit],
  )

  const handleFlatReset = useCallback(() => {
    if (node.type !== 'split') return
    distributePaneSplit(node.id)
  }, [distributePaneSplit, node])

  if (node.type === 'pane') {
    if (hiddenPaneId && node.id === hiddenPaneId) {
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

  if (!flatEntries || flatEntries.length === 0) return null

  const isHorizontal = node.direction === 'horizontal'
  const totalSize = flatEntries.reduce((sum, e) => sum + e.size, 0)
  const handleWidth = 4
  const handleCount = flatEntries.length - 1

  const cssVars = Object.fromEntries(
    flatEntries.map((_, i) => [`--pane-${i}-size`, String((flatEntries[i].size / totalSize) * 100)])
  )

  return (
    <div
      ref={splitContainerRef as React.RefObject<HTMLDivElement>}
      className={`flex h-full w-full ${isHorizontal ? 'flex-row' : 'flex-col'}`}
      style={cssVars as React.CSSProperties}
    >
      {flatEntries.map((entry, index) => {
        const handleDeduction = `${(handleWidth * handleCount) / flatEntries.length}px`
        const entryPosition = childPosition(position, index, flatEntries.length, node.direction)
        const leafNode = entry.node.type === 'pane' ? entry.node : null
        const pane = leafNode ? panes[leafNode.id] : null

        return (
          <div key={entry.node.id} className="contents">
            <div
              className="min-h-0 min-w-0 overflow-hidden"
              style={{
                [isHorizontal ? 'width' : 'height']:
                  `calc(var(--pane-${index}-size) * 1% - ${handleDeduction})`,
              }}
            >
              {entry.node.type === 'split' ? (
                <PaneNodeRenderer
                  node={entry.node}
                  hiddenPaneId={hiddenPaneId}
                  position={entryPosition}
                />
              ) : entry.node.id === hiddenPaneId ? (
                <div className="h-full w-full bg-background" aria-hidden="true" />
              ) : pane ? (
                <PaneBoundary paneId={entry.node.id}>
                  <PaneContainer pane={pane} position={entryPosition} />
                </PaneBoundary>
              ) : null}
            </div>
            {index < flatEntries.length - 1 && (
              <FlatResizeHandle
                direction={node.direction}
                index={index}
                entries={flatEntries}
                splitContainerRef={splitContainerRef}
                onReset={handleFlatReset}
                onResize={handleFlatResize}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}
