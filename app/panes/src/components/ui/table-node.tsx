'use client'

import * as React from 'react'

import { BlockSelectionPlugin, useBlockSelected } from '@platejs/selection/react'
import { getTableColumnCount } from '@platejs/table'
import {
  TablePlugin,
  TableProvider,
  useCellIndices,
  useTableCellBorders,
  useTableColSizes,
  useTableElement,
  useTableSelectionDom,
  useTableValue,
} from '@platejs/table/react'
import { DotsSixVerticalIcon } from '@phosphor-icons/react'
import { type TTableCellElement, type TTableElement, type TTableRowElement, KEYS } from 'platejs'
import {
  type PlateElementProps,
  PlateElement,
  useEditorPlugin,
  useEditorRef,
  useElementSelector,
  usePluginOption,
  useReadOnly,
  withHOC,
} from 'platejs/react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { blockSelectionVariants } from './block-selection'
import { TableFloatingToolbar } from './table-floating-toolbar'
import {
  TABLE_DEFAULT_COLUMN_WIDTH,
  TableResizeContext,
  useTableResizeContext,
  useTableResizeController,
} from './use-table-resize-controller'

const TABLE_CONTROL_COLUMN_WIDTH = 8
const TABLE_DEFERRED_COLUMN_RESIZE_CELL_COUNT = 1200

export const TableElement = withHOC(
  TableProvider,
  function TableElement({ children, ...props }: PlateElementProps<TTableElement>) {
    const readOnly = useReadOnly()
    const isSelectionAreaVisible = usePluginOption(BlockSelectionPlugin, 'isSelectionAreaVisible')
    const hasControls = !readOnly && !isSelectionAreaVisible
    const { marginLeft, props: tableProps } = useTableElement()
    const colSizes = useTableColSizes()
    const controlColumnWidth = hasControls ? TABLE_CONTROL_COLUMN_WIDTH : 0
    const dragIndicatorRef = React.useRef<HTMLDivElement>(null)
    const hoverIndicatorRef = React.useRef<HTMLDivElement>(null)
    const deferColumnResize =
      colSizes.length * props.element.children.length > TABLE_DEFERRED_COLUMN_RESIZE_CELL_COUNT
    const tablePath = useElementSelector(([, path]) => path, [], {
      key: KEYS.table,
    })
    const tableRef = React.useRef<HTMLTableElement>(null)
    const wrapperRef = React.useRef<HTMLDivElement>(null)
    useTableSelectionDom(tableRef)
    const resizeController = useTableResizeController({
      controlColumnWidth,
      deferColumnResize,
      dragIndicatorRef,
      hoverIndicatorRef,
      marginLeft,
      tablePath,
      tableRef,
      wrapperRef,
    })
    const resolvedColSizes = React.useMemo(() => {
      if (colSizes.length > 0) {
        return colSizes.map((colSize) => colSize || TABLE_DEFAULT_COLUMN_WIDTH)
      }

      return Array.from(
        { length: getTableColumnCount(props.element) },
        () => TABLE_DEFAULT_COLUMN_WIDTH,
      )
    }, [colSizes, props.element])
    const tableVariableStyle = React.useMemo(() => {
      if (resolvedColSizes.length === 0) {
        return
      }

      return {
        ...Object.fromEntries(
          resolvedColSizes.map((colSize, index) => [`--table-col-${index}`, `${colSize}px`]),
        ),
      } as React.CSSProperties
    }, [resolvedColSizes])
    const tableStyle = React.useMemo(
      () =>
        ({
          width: `${
            resolvedColSizes.reduce((total, colSize) => total + colSize, 0) + controlColumnWidth
          }px`,
        }) as React.CSSProperties,
      [controlColumnWidth, resolvedColSizes],
    )

    const isSelectingTable = useBlockSelected(props.element.id as string)

    const content = (
      <PlateElement
        {...props}
        className={cn(
          'overflow-x-auto py-5',
          hasControls && '-ml-2 *:data-[slot=block-selection]:left-2',
        )}
        style={{ paddingLeft: marginLeft }}
      >
        <TableResizeContext.Provider value={resizeController}>
          <div ref={wrapperRef} className="group/table relative w-fit" style={tableVariableStyle}>
            <div
              ref={dragIndicatorRef}
              className="-translate-x-[1.5px] pointer-events-none absolute inset-y-0 z-36 hidden w-[3px] bg-ring/70"
              contentEditable={false}
            />
            <div
              ref={hoverIndicatorRef}
              className="-translate-x-[1.5px] pointer-events-none absolute inset-y-0 z-35 hidden w-[3px] bg-ring/80"
              contentEditable={false}
            />
            <table
              ref={tableRef}
              className={cn(
                'mr-0 ml-px table h-px table-fixed border-collapse',
                'data-[table-selecting=true]:[&_*::selection]:!bg-transparent',
                'data-[table-selecting=true]:[&_*::selection]:!text-inherit',
                'data-[table-selecting=true]:[&_*::-moz-selection]:!bg-transparent',
                'data-[table-selecting=true]:[&_*::-moz-selection]:!text-inherit',
                'data-[table-selecting=true]:[&_*]:!caret-transparent',
              )}
              style={tableStyle}
              {...tableProps}
            >
              {resolvedColSizes.length > 0 && (
                <colgroup>
                  {hasControls && (
                    <col
                      style={{
                        maxWidth: TABLE_CONTROL_COLUMN_WIDTH,
                        minWidth: TABLE_CONTROL_COLUMN_WIDTH,
                        width: TABLE_CONTROL_COLUMN_WIDTH,
                      }}
                    />
                  )}
                  {resolvedColSizes.map((colSize, index) => (
                    <col
                      key={index}
                      style={{
                        maxWidth: colSize,
                        minWidth: colSize,
                        width: colSize,
                      }}
                    />
                  ))}
                </colgroup>
              )}
              <tbody className="min-w-full">{children}</tbody>
            </table>

            {isSelectingTable && (
              <div className={blockSelectionVariants()} contentEditable={false} />
            )}
          </div>
        </TableResizeContext.Provider>
      </PlateElement>
    )

    if (readOnly) {
      return content
    }

    return <TableFloatingToolbar>{content}</TableFloatingToolbar>
  },
)

export function TableRowElement({ children, ...props }: PlateElementProps<TTableRowElement>) {
  const { element } = props
  const readOnly = useReadOnly()
  const editor = useEditorRef()
  const rowIndex = useElementSelector(([, path]) => path.at(-1) as number, [], {
    key: KEYS.tr,
  })
  const rowSize = useElementSelector(([node]) => (node as TTableRowElement).size, [], {
    key: KEYS.tr,
  })
  const rowSizeOverrides = useTableValue('rowSizeOverrides')
  const rowMinHeight = rowSizeOverrides.get?.(rowIndex) ?? rowSize
  const isSelectionAreaVisible = usePluginOption(BlockSelectionPlugin, 'isSelectionAreaVisible')
  const hasControls = !readOnly && !isSelectionAreaVisible

  return (
    <PlateElement
      {...props}
      as="tr"
      className="group/row"
      style={
        {
          ...props.style,
          '--tableRowMinHeight': rowMinHeight ? `${rowMinHeight}px` : undefined,
        } as React.CSSProperties
      }
    >
      {hasControls && (
        <td className="w-2 min-w-2 max-w-2 select-none p-0" contentEditable={false}>
          <RowSelectHandle onSelect={() => editor.tf.select(element)} />
        </td>
      )}

      {children}
    </PlateElement>
  )
}

function useTableCellPresentation(element: TTableCellElement) {
  const { api } = useEditorPlugin(TablePlugin)
  const borders = useTableCellBorders({ element })
  const { col, row } = useCellIndices()

  const colSpan = api.table.getColSpan(element)
  const rowSpan = api.table.getRowSpan(element)
  const width = React.useMemo(() => {
    const terms = Array.from(
      { length: colSpan },
      (_, offset) => `var(--table-col-${col + offset}, 120px)`,
    )

    return terms.length === 1 ? terms[0]! : `calc(${terms.join(' + ')})`
  }, [col, colSpan])

  return {
    borders,
    colIndex: col + colSpan - 1,
    colSpan,
    rowIndex: row + rowSpan - 1,
    rowSpan,
    width,
  }
}

// Selects the row. Rows were never draggable in the file editor: the drag
// plumbing this handle once carried needed a Plate dnd plugin the editor never
// registered.
function RowSelectHandle({ onSelect }: { onSelect: () => void }) {
  return (
    <Button
      variant="outline"
      aria-label="Select row"
      className={cn(
        '-translate-y-1/2 absolute top-1/2 left-0 z-51 h-6 w-4 p-0 focus-visible:ring-0 focus-visible:ring-offset-0',
        'opacity-0 transition-opacity duration-100 group-hover/row:opacity-100 group-data-[table-resizing=true]/row:opacity-0',
      )}
      onClick={onSelect}
    >
      <DotsSixVerticalIcon className="text-muted-foreground" />
    </Button>
  )
}

export function TableCellElement({
  isHeader,
  ...props
}: PlateElementProps<TTableCellElement> & {
  isHeader?: boolean
}) {
  const readOnly = useReadOnly()
  const element = props.element

  const tableId = useElementSelector(([node]) => node.id as string, [], {
    key: KEYS.table,
  })
  const rowId = useElementSelector(([node]) => node.id as string, [], {
    key: KEYS.tr,
  })
  const isSelectingTable = useBlockSelected(tableId)
  const isSelectingRow = useBlockSelected(rowId) || isSelectingTable
  const isSelectionAreaVisible = usePluginOption(BlockSelectionPlugin, 'isSelectionAreaVisible')

  const { borders, colIndex, colSpan, rowIndex, rowSpan, width } = useTableCellPresentation(element)

  return (
    <PlateElement
      {...props}
      as={isHeader ? 'th' : 'td'}
      className={cn(
        'relative h-full overflow-visible border-none bg-background p-0',
        element.background ? 'bg-(--cellBackground)' : 'bg-background',
        isHeader && 'text-left *:m-0',
        'before:size-full',
        'data-[table-cell-selected=true]:before:z-10',
        'data-[table-cell-selected=true]:before:bg-brand/5',
        "before:absolute before:box-border before:select-none before:content-['']",
        borders.bottom?.size && 'before:border-b before:border-b-border',
        borders.right?.size && 'before:border-r before:border-r-border',
        borders.left?.size && 'before:border-l before:border-l-border',
        borders.top?.size && 'before:border-t before:border-t-border',
      )}
      style={
        {
          '--cellBackground': element.background,
          maxWidth: width,
          minWidth: width,
        } as React.CSSProperties
      }
      attributes={{
        ...props.attributes,
        colSpan,
        'data-table-cell-id': element.id,
        rowSpan,
      }}
    >
      <div
        className="relative z-20 box-border h-full px-3 py-2"
        style={rowSpan === 1 ? { minHeight: 'var(--tableRowMinHeight, 0px)' } : undefined}
      >
        {props.children}
      </div>

      {!readOnly && !isSelectionAreaVisible && (
        <TableCellResizeControls colIndex={colIndex} rowIndex={rowIndex} />
      )}

      {isSelectingRow && <div className={blockSelectionVariants()} contentEditable={false} />}
    </PlateElement>
  )
}

export function TableCellHeaderElement(props: React.ComponentProps<typeof TableCellElement>) {
  return <TableCellElement {...props} isHeader />
}

const TableCellResizeControls = React.memo(function TableCellResizeControls({
  colIndex,
  rowIndex,
}: {
  colIndex: number
  rowIndex: number
}) {
  const { clearResizePreview, disableMarginLeft, setResizePreview, startResize } =
    useTableResizeContext()
  const rightHandleKey = `right:${rowIndex}:${colIndex}`
  const bottomHandleKey = `bottom:${rowIndex}:${colIndex}`
  const leftHandleKey = `left:${rowIndex}:${colIndex}`
  const isLeftHandle = colIndex === 0 && !disableMarginLeft

  return (
    <div
      className="group/resize pointer-events-none absolute inset-0 z-30 select-none"
      contentEditable={false}
      suppressContentEditableWarning={true}
    >
      <div
        className="-top-2 -right-1 pointer-events-auto absolute z-40 h-[calc(100%_+_8px)] w-2 cursor-col-resize touch-none"
        onPointerEnter={(event) => {
          setResizePreview(event, {
            colIndex,
            direction: 'right',
            handleKey: rightHandleKey,
            rowIndex,
          })
        }}
        onPointerLeave={() => {
          clearResizePreview(rightHandleKey)
        }}
        onPointerDown={(event) => {
          startResize(event, {
            colIndex,
            direction: 'right',
            handleKey: rightHandleKey,
            rowIndex,
          })
        }}
      />
      <div
        className="-bottom-1 pointer-events-auto absolute left-0 z-40 h-2 w-full cursor-row-resize touch-none"
        onPointerEnter={(event) => {
          setResizePreview(event, {
            colIndex,
            direction: 'bottom',
            handleKey: bottomHandleKey,
            rowIndex,
          })
        }}
        onPointerLeave={() => {
          clearResizePreview(bottomHandleKey)
        }}
        onPointerDown={(event) => {
          startResize(event, {
            colIndex,
            direction: 'bottom',
            handleKey: bottomHandleKey,
            rowIndex,
          })
        }}
      />
      {isLeftHandle && (
        <div
          className="-left-1 pointer-events-auto absolute top-0 z-40 h-full w-2 cursor-col-resize touch-none"
          onPointerEnter={(event) => {
            setResizePreview(event, {
              colIndex,
              direction: 'left',
              handleKey: leftHandleKey,
              rowIndex,
            })
          }}
          onPointerLeave={() => {
            clearResizePreview(leftHandleKey)
          }}
          onPointerDown={(event) => {
            startResize(event, {
              colIndex,
              direction: 'left',
              handleKey: leftHandleKey,
              rowIndex,
            })
          }}
        />
      )}
    </div>
  )
})

TableCellResizeControls.displayName = 'TableCellResizeControls'
