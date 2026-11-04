'use client'

import * as React from 'react'

import {
  TablePlugin,
  useTableBordersDropdownMenuContentState,
  useTableMergeState,
} from '@platejs/table/react'
import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowUpIcon,
  GridFourIcon,
  PaintBucketIcon,
  SquareSplitHorizontalIcon,
  TrashIcon,
  UniteIcon,
  XIcon,
} from '@phosphor-icons/react'
import type { TTableElement } from 'platejs'
import {
  useEditorPlugin,
  useEditorRef,
  useEditorSelector,
  useElement,
  useFocusedLast,
  useRemoveNodeButton,
  useSelected,
} from 'platejs/react'
import { Popover as PopoverPrimitive } from '@base-ui/react/popover'
import { useRender } from '@base-ui/react/use-render'

import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { TableCellColorMenu } from './table-cell-color-menu'
import {
  BorderAllIcon,
  BorderBottomIcon,
  BorderLeftIcon,
  BorderNoneIcon,
  BorderRightIcon,
  BorderTopIcon,
} from './table-icons'
import { Toolbar, ToolbarButton, ToolbarGroup } from './toolbar'

const TABLE_MULTI_SELECTION_TOOLBAR_DELAY_MS = 150

// The floating table toolbar has no trigger: it anchors to the table itself.
// base-ui anchors through the Positioner's `anchor` prop, so the table
// registers its element here and the toolbar positions against it.
const TableToolbarAnchor = React.createContext<Element | null>(null)

function PopoverContent({ children }: { children: React.ReactElement }) {
  const anchor = React.useContext(TableToolbarAnchor)
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Positioner anchor={anchor} className="z-50">
        <PopoverPrimitive.Popup
          className="outline-none"
          initialFocus={false}
          finalFocus={false}
          render={children}
        />
      </PopoverPrimitive.Positioner>
    </PopoverPrimitive.Portal>
  )
}

/** The table's own toolbar: cell controls for a caret in one cell, merge/split for a range. */
export function TableFloatingToolbar({ children }: { children: React.ReactElement }) {
  const selectedCellCount = useEditorSelector(
    (editor) => editor.getApi(TablePlugin).table.getSelectedCellIds()?.length ?? 0,
    [],
  )
  const selected = useSelected()
  const isFocusedLast = useFocusedLast()
  const [isExpandedSelectionToolbarReady, setIsExpandedSelectionToolbarReady] =
    React.useState(false)
  // `getSelectedCellIds` only reports cells once a range spans more than one
  // cell, so a count of zero means the selection (a caret or an expanded
  // range) is confined to a single cell. Gating on it instead of
  // `editor.api.isCollapsed()` keeps the row/column controls available while a
  // cell's content is fully selected (e.g. select-all inside a cell), not just
  // while the caret is collapsed in it.
  const isSingleCellToolbarOpen = isFocusedLast && selected && selectedCellCount === 0
  const isExpandedSelectionPending = isFocusedLast && selectedCellCount > 1

  React.useEffect(() => {
    if (!isExpandedSelectionPending) {
      // Reset the delayed toolbar gate when selection is no longer expanded.
      setIsExpandedSelectionToolbarReady(false)

      return
    }

    const timeoutId = window.setTimeout(() => {
      setIsExpandedSelectionToolbarReady(true)
    }, TABLE_MULTI_SELECTION_TOOLBAR_DELAY_MS)

    return () => {
      window.clearTimeout(timeoutId)
    }
  }, [isExpandedSelectionPending])

  const shouldRenderExpandedSelectionToolbar =
    isExpandedSelectionToolbarReady && isExpandedSelectionPending
  const isToolbarOpen = isSingleCellToolbarOpen || shouldRenderExpandedSelectionToolbar

  const [anchor, setAnchor] = React.useState<Element | null>(null)
  const anchoredTable = useRender({ render: children, ref: setAnchor })

  return (
    <TableToolbarAnchor.Provider value={anchor}>
      {anchoredTable}
      <PopoverPrimitive.Root open={isToolbarOpen} modal={false}>
        {isSingleCellToolbarOpen && <SingleCellTableFloatingToolbarContent />}
        {shouldRenderExpandedSelectionToolbar && <ExpandedSelectionTableFloatingToolbarContent />}
      </PopoverPrimitive.Root>
    </TableToolbarAnchor.Provider>
  )
}

function ExpandedSelectionTableFloatingToolbarContent() {
  const { tf } = useEditorPlugin(TablePlugin)
  const { canMerge, canSplit } = useTableMergeState()

  if (!canMerge && !canSplit) return null

  return (
    <TableFloatingToolbarContent
      canMerge={canMerge}
      canSplit={canSplit}
      onMerge={() => tf.table.merge()}
      onSplit={() => tf.table.split()}
    />
  )
}

function SingleCellTableFloatingToolbarContent() {
  const { tf } = useEditorPlugin(TablePlugin)
  const element = useElement<TTableElement>()
  const { props: buttonProps } = useRemoveNodeButton({ element })
  const { canSplit } = useTableMergeState()

  return (
    <TableFloatingToolbarContent
      buttonProps={buttonProps}
      canSplit={canSplit}
      singleCellMode
      onDeleteColumn={() => {
        tf.remove.tableColumn()
      }}
      onDeleteRow={() => {
        tf.remove.tableRow()
      }}
      onInsertColumnAfter={() => {
        tf.insert.tableColumn()
      }}
      onInsertColumnBefore={() => {
        tf.insert.tableColumn({ before: true })
      }}
      onInsertRowAfter={() => {
        tf.insert.tableRow()
      }}
      onInsertRowBefore={() => {
        tf.insert.tableRow({ before: true })
      }}
      onSplit={() => tf.table.split()}
    />
  )
}

function TableFloatingToolbarContent({
  buttonProps,
  canMerge = false,
  canSplit = false,
  singleCellMode = false,
  onDeleteColumn,
  onDeleteRow,
  onInsertColumnAfter,
  onInsertColumnBefore,
  onInsertRowAfter,
  onInsertRowBefore,
  onMerge,
  onSplit,
}: {
  buttonProps?: React.ComponentProps<typeof ToolbarButton>
  canMerge?: boolean
  canSplit?: boolean
  singleCellMode?: boolean
  onDeleteColumn?: () => void
  onDeleteRow?: () => void
  onInsertColumnAfter?: () => void
  onInsertColumnBefore?: () => void
  onInsertRowAfter?: () => void
  onInsertRowBefore?: () => void
  onMerge?: () => void
  onSplit?: () => void
}) {
  return (
    <PopoverContent>
      <Toolbar
        className="flex w-auto max-w-[80vw] flex-row overflow-x-auto rounded-md border bg-popover p-1 shadow-md print:hidden [&::-webkit-scrollbar]:hidden [scrollbar-width:none]"
        contentEditable={false}
      >
        <ToolbarGroup>
          <TableCellColorMenu tooltip="Background color">
            <PaintBucketIcon />
          </TableCellColorMenu>
          {canMerge && onMerge && (
            <ToolbarButton
              onClick={onMerge}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Merge cells"
            >
              <UniteIcon />
            </ToolbarButton>
          )}
          {canSplit && onSplit && (
            <ToolbarButton
              onClick={onSplit}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Split cell"
            >
              <SquareSplitHorizontalIcon />
            </ToolbarButton>
          )}

          <DropdownMenu modal={false}>
            <DropdownMenuTrigger render={<ToolbarButton tooltip="Cell borders" />}>
              <GridFourIcon />
            </DropdownMenuTrigger>

            <DropdownMenuPortal>
              <TableBordersDropdownMenuContent />
            </DropdownMenuPortal>
          </DropdownMenu>

          {singleCellMode && (
            <ToolbarGroup>
              <ToolbarButton tooltip="Delete table" {...buttonProps}>
                <TrashIcon />
              </ToolbarButton>
            </ToolbarGroup>
          )}
        </ToolbarGroup>
        {singleCellMode && (
          <ToolbarGroup>
            <ToolbarButton
              onClick={onInsertRowBefore}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Insert row before"
            >
              <ArrowUpIcon />
            </ToolbarButton>
            <ToolbarButton
              onClick={onInsertRowAfter}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Insert row after"
            >
              <ArrowDownIcon />
            </ToolbarButton>
            <ToolbarButton
              onClick={onDeleteRow}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Delete row"
            >
              <XIcon />
            </ToolbarButton>
          </ToolbarGroup>
        )}
        {singleCellMode && (
          <ToolbarGroup>
            <ToolbarButton
              onClick={onInsertColumnBefore}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Insert column before"
            >
              <ArrowLeftIcon />
            </ToolbarButton>
            <ToolbarButton
              onClick={onInsertColumnAfter}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Insert column after"
            >
              <ArrowRightIcon />
            </ToolbarButton>
            <ToolbarButton
              onClick={onDeleteColumn}
              onMouseDown={(e) => e.preventDefault()}
              tooltip="Delete column"
            >
              <XIcon />
            </ToolbarButton>
          </ToolbarGroup>
        )}
      </Toolbar>
    </PopoverContent>
  )
}

function TableBordersDropdownMenuContent(props: React.ComponentProps<typeof DropdownMenuContent>) {
  const editor = useEditorRef()
  const {
    getOnSelectTableBorder,
    hasBottomBorder,
    hasLeftBorder,
    hasNoBorders,
    hasOuterBorders,
    hasRightBorder,
    hasTopBorder,
  } = useTableBordersDropdownMenuContentState()

  return (
    <DropdownMenuContent
      className="min-w-[220px]"
      finalFocus={() => {
        editor.tf.focus()
        return false
      }}
      align="start"
      side="right"
      sideOffset={0}
      {...props}
    >
      <DropdownMenuGroup>
        <DropdownMenuCheckboxItem
          checked={hasTopBorder}
          onCheckedChange={getOnSelectTableBorder('top')}
        >
          <BorderTopIcon />
          <div>Top Border</div>
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={hasRightBorder}
          onCheckedChange={getOnSelectTableBorder('right')}
        >
          <BorderRightIcon />
          <div>Right Border</div>
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={hasBottomBorder}
          onCheckedChange={getOnSelectTableBorder('bottom')}
        >
          <BorderBottomIcon />
          <div>Bottom Border</div>
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={hasLeftBorder}
          onCheckedChange={getOnSelectTableBorder('left')}
        >
          <BorderLeftIcon />
          <div>Left Border</div>
        </DropdownMenuCheckboxItem>
      </DropdownMenuGroup>

      <DropdownMenuGroup>
        <DropdownMenuCheckboxItem
          checked={hasNoBorders}
          onCheckedChange={getOnSelectTableBorder('none')}
        >
          <BorderNoneIcon />
          <div>No Border</div>
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={hasOuterBorders}
          onCheckedChange={getOnSelectTableBorder('outer')}
        >
          <BorderAllIcon />
          <div>Outside Borders</div>
        </DropdownMenuCheckboxItem>
      </DropdownMenuGroup>
    </DropdownMenuContent>
  )
}
