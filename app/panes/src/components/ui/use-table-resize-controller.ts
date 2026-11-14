import * as React from 'react'

import { resizeLengthClampStatic } from '@platejs/resizable'
import { setTableColSize, setTableMarginLeft, setTableRowSize } from '@platejs/table'
import {
  TablePlugin,
  roundCellSizeToStep,
  useOverrideColSize,
  useOverrideMarginLeft,
  useOverrideRowSize,
  useTableColSizes,
} from '@platejs/table/react'
import { useEditorPlugin } from 'platejs/react'

type TableResizeDirection = 'bottom' | 'left' | 'right'

type TableResizeStartOptions = {
  colIndex: number
  direction: TableResizeDirection
  handleKey: string
  rowIndex: number
}

type TableResizeDragState = {
  colIndex: number
  direction: TableResizeDirection
  initialPosition: number
  initialSize: number
  marginLeft: number
  rowIndex: number
}

export const TABLE_DEFAULT_COLUMN_WIDTH = 120

export const TableResizeContext = React.createContext<ReturnType<
  typeof useTableResizeController
> | null>(null)

export function useTableResizeContext() {
  const context = React.useContext(TableResizeContext)

  if (!context) {
    throw new Error('TableResizeContext is missing')
  }

  return context
}

/** Column, row and left-margin drags on one table: previews while the pointer moves, commits on release. */
export function useTableResizeController({
  deferColumnResize,
  dragIndicatorRef,
  hoverIndicatorRef,
  marginLeft,
  controlColumnWidth,
  tablePath,
  tableRef,
  wrapperRef,
}: {
  deferColumnResize: boolean
  dragIndicatorRef: React.RefObject<HTMLDivElement | null>
  hoverIndicatorRef: React.RefObject<HTMLDivElement | null>
  marginLeft: number
  controlColumnWidth: number
  tablePath: number[]
  tableRef: React.RefObject<HTMLTableElement | null>
  wrapperRef: React.RefObject<HTMLDivElement | null>
}) {
  const { editor, getOptions } = useEditorPlugin(TablePlugin)
  const { disableMarginLeft = false, minColumnWidth = 0 } = getOptions()
  const colSizes = useTableColSizes({
    disableOverrides: true,
  })
  const effectiveColSizes = React.useMemo(
    () => colSizes.map((colSize) => colSize || TABLE_DEFAULT_COLUMN_WIDTH),
    [colSizes],
  )
  const effectiveColSizesRef = React.useRef(effectiveColSizes)
  const activeHandleKeyRef = React.useRef<string | null>(null)
  const activeRowElementRef = React.useRef<HTMLTableRowElement | null>(null)
  const cleanupListenersRef = React.useRef<(() => void) | null>(null)
  const marginLeftRef = React.useRef(marginLeft)
  const dragStateRef = React.useRef<TableResizeDragState | null>(null)
  const frozenRowIndicesRef = React.useRef<number[] | null>(null)
  const previewHandleKeyRef = React.useRef<string | null>(null)
  const overrideColSize = useOverrideColSize()
  const overrideMarginLeft = useOverrideMarginLeft()
  const overrideRowSize = useOverrideRowSize()

  React.useEffect(() => {
    effectiveColSizesRef.current = effectiveColSizes
  }, [effectiveColSizes])

  React.useEffect(() => {
    marginLeftRef.current = marginLeft
  }, [marginLeft])

  const hideDeferredResizeIndicator = React.useCallback(() => {
    const indicator = dragIndicatorRef.current

    if (!indicator) return

    indicator.style.display = 'none'
    indicator.style.removeProperty('left')
  }, [dragIndicatorRef])

  const showDeferredResizeIndicator = React.useCallback(
    (offset: number) => {
      const indicator = dragIndicatorRef.current

      if (!indicator) return

      indicator.style.display = 'block'
      indicator.style.left = `${offset}px`
    },
    [dragIndicatorRef],
  )

  const hideResizeIndicator = React.useCallback(() => {
    const indicator = hoverIndicatorRef.current

    if (!indicator) return

    indicator.style.display = 'none'
    indicator.style.removeProperty('left')
  }, [hoverIndicatorRef])

  const clearFrozenRowHeights = React.useCallback(() => {
    const frozenRowIndices = frozenRowIndicesRef.current

    if (!frozenRowIndices) return

    frozenRowIndicesRef.current = null

    frozenRowIndices.forEach((rowIndex) => {
      overrideRowSize(rowIndex, null)
    })
  }, [overrideRowSize])

  const freezeRowHeights = React.useCallback(() => {
    const table = tableRef.current

    if (!table || deferColumnResize) return

    clearFrozenRowHeights()

    const frozenRowIndices: number[] = []

    Array.from(table.rows).forEach((row, rowIndex) => {
      const height = row.getBoundingClientRect().height

      if (!height) return

      overrideRowSize(rowIndex, height)
      frozenRowIndices.push(rowIndex)
    })

    frozenRowIndicesRef.current = frozenRowIndices
  }, [clearFrozenRowHeights, deferColumnResize, overrideRowSize, tableRef])

  const showResizeIndicatorAtOffset = React.useCallback(
    (offset: number) => {
      const indicator = hoverIndicatorRef.current

      if (!indicator) return

      indicator.style.display = 'block'
      indicator.style.left = `${offset}px`
    },
    [hoverIndicatorRef],
  )

  const showResizeIndicator = React.useCallback(
    ({
      event,
      direction,
    }: Pick<TableResizeStartOptions, 'direction'> & {
      event: React.PointerEvent<HTMLDivElement>
    }) => {
      if (direction === 'bottom') return

      const wrapper = wrapperRef.current

      if (!wrapper) return

      const handleRect = event.currentTarget.getBoundingClientRect()
      const wrapperRect = wrapper.getBoundingClientRect()
      const boundaryOffset = handleRect.left - wrapperRect.left + handleRect.width / 2

      showResizeIndicatorAtOffset(boundaryOffset)
    },
    [showResizeIndicatorAtOffset, wrapperRef],
  )

  const setResizePreview = React.useCallback(
    (event: React.PointerEvent<HTMLDivElement>, options: TableResizeStartOptions) => {
      if (activeHandleKeyRef.current) return

      previewHandleKeyRef.current = options.handleKey
      showResizeIndicator({ ...options, event })
    },
    [showResizeIndicator],
  )

  const clearResizePreview = React.useCallback(
    (handleKey: string) => {
      if (activeHandleKeyRef.current) return
      if (previewHandleKeyRef.current !== handleKey) return

      previewHandleKeyRef.current = null
      hideResizeIndicator()
    },
    [hideResizeIndicator],
  )

  const commitColSize = React.useCallback(
    (colIndex: number, width: number) => {
      setTableColSize(editor, { colIndex, width }, { at: tablePath })
      setTimeout(() => overrideColSize(colIndex, null), 0)
    },
    [editor, overrideColSize, tablePath],
  )

  const commitRowSize = React.useCallback(
    (rowIndex: number, height: number) => {
      setTableRowSize(editor, { height, rowIndex }, { at: tablePath })
      setTimeout(() => overrideRowSize(rowIndex, null), 0)
    },
    [editor, overrideRowSize, tablePath],
  )

  const commitMarginLeft = React.useCallback(
    (nextMarginLeft: number) => {
      setTableMarginLeft(editor, { marginLeft: nextMarginLeft }, { at: tablePath })
      setTimeout(() => overrideMarginLeft(null), 0)
    },
    [editor, overrideMarginLeft, tablePath],
  )

  const getColumnBoundaryOffset = React.useCallback(
    (colIndex: number, currentWidth: number) =>
      controlColumnWidth +
      effectiveColSizesRef.current
        .slice(0, colIndex)
        .reduce((total, colSize) => total + colSize, 0) +
      currentWidth,
    [controlColumnWidth],
  )

  const applyResize = React.useCallback(
    (event: PointerEvent, finished: boolean) => {
      const dragState = dragStateRef.current

      if (!dragState) return

      const currentPosition = dragState.direction === 'bottom' ? event.clientY : event.clientX
      const delta = currentPosition - dragState.initialPosition

      if (dragState.direction === 'bottom') {
        const newHeight = roundCellSizeToStep(dragState.initialSize + delta, undefined)

        if (finished) {
          commitRowSize(dragState.rowIndex, newHeight)
        } else {
          overrideRowSize(dragState.rowIndex, newHeight)
        }

        return
      }

      if (dragState.direction === 'left') {
        const initial = effectiveColSizesRef.current[dragState.colIndex] ?? dragState.initialSize
        const complement = (width: number) => initial + dragState.marginLeft - width
        const nextMarginLeft = roundCellSizeToStep(
          resizeLengthClampStatic(dragState.marginLeft + delta, {
            max: complement(minColumnWidth),
            min: 0,
          }),
          undefined,
        )
        const nextWidth = complement(nextMarginLeft)

        if (finished) {
          commitMarginLeft(nextMarginLeft)
          commitColSize(dragState.colIndex, nextWidth)
        } else if (deferColumnResize) {
          showDeferredResizeIndicator(controlColumnWidth + (nextMarginLeft - dragState.marginLeft))
        } else {
          showResizeIndicatorAtOffset(controlColumnWidth + (nextMarginLeft - dragState.marginLeft))
          overrideMarginLeft(nextMarginLeft)
          overrideColSize(dragState.colIndex, nextWidth)
        }

        return
      }

      const currentInitial =
        effectiveColSizesRef.current[dragState.colIndex] ?? dragState.initialSize
      const nextInitial = effectiveColSizesRef.current[dragState.colIndex + 1]
      const complement = (width: number) => currentInitial + nextInitial - width
      const currentWidth = roundCellSizeToStep(
        resizeLengthClampStatic(currentInitial + delta, {
          max: nextInitial ? complement(minColumnWidth) : undefined,
          min: minColumnWidth,
        }),
        undefined,
      )
      const nextWidth = nextInitial ? complement(currentWidth) : undefined

      if (finished) {
        commitColSize(dragState.colIndex, currentWidth)

        if (nextWidth !== undefined) {
          commitColSize(dragState.colIndex + 1, nextWidth)
        }
      } else if (deferColumnResize) {
        showDeferredResizeIndicator(getColumnBoundaryOffset(dragState.colIndex, currentWidth))
      } else {
        showResizeIndicatorAtOffset(getColumnBoundaryOffset(dragState.colIndex, currentWidth))
        overrideColSize(dragState.colIndex, currentWidth)

        if (nextWidth !== undefined) {
          overrideColSize(dragState.colIndex + 1, nextWidth)
        }
      }
    },
    [
      commitColSize,
      commitMarginLeft,
      commitRowSize,
      controlColumnWidth,
      deferColumnResize,
      getColumnBoundaryOffset,
      showDeferredResizeIndicator,
      showResizeIndicatorAtOffset,
      minColumnWidth,
      overrideColSize,
      overrideMarginLeft,
      overrideRowSize,
    ],
  )

  const stopResize = React.useCallback(() => {
    cleanupListenersRef.current?.()
    cleanupListenersRef.current = null
    activeHandleKeyRef.current = null
    previewHandleKeyRef.current = null
    dragStateRef.current = null

    if (activeRowElementRef.current) {
      delete activeRowElementRef.current.dataset.tableResizing
      activeRowElementRef.current = null
    }

    hideDeferredResizeIndicator()
    hideResizeIndicator()
    clearFrozenRowHeights()
  }, [clearFrozenRowHeights, hideDeferredResizeIndicator, hideResizeIndicator])

  React.useEffect(() => stopResize, [stopResize])

  const startResize = React.useCallback(
    (
      event: React.PointerEvent<HTMLDivElement>,
      { colIndex, direction, handleKey, rowIndex }: TableResizeStartOptions,
    ) => {
      const rowHeight = tableRef.current?.rows.item(rowIndex)?.getBoundingClientRect().height ?? 0

      dragStateRef.current = {
        colIndex,
        direction,
        initialPosition: direction === 'bottom' ? event.clientY : event.clientX,
        initialSize:
          direction === 'bottom'
            ? rowHeight
            : (effectiveColSizesRef.current[colIndex] ?? TABLE_DEFAULT_COLUMN_WIDTH),
        marginLeft: marginLeftRef.current,
        rowIndex,
      }
      activeHandleKeyRef.current = handleKey
      previewHandleKeyRef.current = null

      const rowElement = tableRef.current?.rows.item(rowIndex) ?? null

      if (activeRowElementRef.current && activeRowElementRef.current !== rowElement) {
        delete activeRowElementRef.current.dataset.tableResizing
      }

      activeRowElementRef.current = rowElement

      if (rowElement) {
        rowElement.dataset.tableResizing = 'true'
      }

      cleanupListenersRef.current?.()

      if (direction !== 'bottom') {
        freezeRowHeights()
      }

      const handlePointerMove = (pointerEvent: PointerEvent) => {
        applyResize(pointerEvent, false)
      }

      const handlePointerEnd = (pointerEvent: PointerEvent) => {
        applyResize(pointerEvent, true)
        stopResize()
      }

      // react-doctor-disable-next-line effect-needs-cleanup -- this is a pointerdown handler, not an effect; the three listeners are torn down by `cleanupListenersRef.current` (assigned immediately below), which `stopResize` invokes on pointerup/pointercancel and which the unmount effect (`React.useEffect(() => stopResize, [stopResize])`) also runs. The tracer can't follow that ref indirection.
      window.addEventListener('pointermove', handlePointerMove)
      window.addEventListener('pointerup', handlePointerEnd)
      window.addEventListener('pointercancel', handlePointerEnd)

      cleanupListenersRef.current = () => {
        window.removeEventListener('pointermove', handlePointerMove)
        window.removeEventListener('pointerup', handlePointerEnd)
        window.removeEventListener('pointercancel', handlePointerEnd)
      }

      if (deferColumnResize && direction !== 'bottom') {
        hideResizeIndicator()
        showDeferredResizeIndicator(
          direction === 'left'
            ? controlColumnWidth
            : getColumnBoundaryOffset(
                colIndex,
                effectiveColSizesRef.current[colIndex] ?? TABLE_DEFAULT_COLUMN_WIDTH,
              ),
        )
      } else {
        showResizeIndicator({ direction, event })
      }

      event.preventDefault()
      event.stopPropagation()
    },
    [
      controlColumnWidth,
      deferColumnResize,
      getColumnBoundaryOffset,
      hideResizeIndicator,
      showDeferredResizeIndicator,
      showResizeIndicator,
      stopResize,
      tableRef,
      applyResize,
      freezeRowHeights,
    ],
  )

  return React.useMemo(
    () => ({
      clearResizePreview,
      disableMarginLeft,
      setResizePreview,
      startResize,
    }),
    [clearResizePreview, disableMarginLeft, setResizePreview, startResize],
  )
}
