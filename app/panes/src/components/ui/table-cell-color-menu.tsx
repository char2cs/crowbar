'use client'

import * as React from 'react'

import { setCellBackground } from '@platejs/table'
import { TablePlugin } from '@platejs/table/react'
import { EraserIcon } from '@phosphor-icons/react'
import { useEditorRef } from 'platejs/react'

import { buttonVariants } from '@/components/ui/button-variants'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { WithTooltip, tooltipContentPrimary } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { TABLE_CELL_COLORS } from './table-cell-colors'
import { ToolbarButton, ToolbarMenuGroup } from './toolbar'

function ColorSwatch({
  isBrightColor,
  name,
  value,
  onPick,
}: {
  isBrightColor: boolean
  name: string
  value: string
  onPick: (color: string) => void
}) {
  // The tooltip merges onto the menu item itself (no wrapper), so the item
  // keeps the `role="menuitem"` the surrounding menu drives.
  return (
    <WithTooltip
      trigger={
        <DropdownMenuItem
          className={cn(
            buttonVariants({ size: 'icon', variant: 'outline' }),
            'my-1 flex size-6 items-center justify-center rounded-full border border-muted border-solid p-0 transition-all hover:scale-125',
            !isBrightColor && 'border-transparent text-white',
          )}
          style={{ backgroundColor: value }}
          onSelect={(e) => {
            e.preventDefault()
            onPick(value)
          }}
        />
      }
      content={name}
      sideOffset={4}
      delay={700}
      popupClassName={tooltipContentPrimary}
      className="mb-1 capitalize"
    />
  )
}

/** Sets the background of the selected cells, or clears it. */
export function TableCellColorMenu({
  children,
  tooltip,
}: {
  children: React.ReactNode
  tooltip: string
}) {
  const [open, setOpen] = React.useState(false)
  const editor = useEditorRef()

  const paint = (color: string | null) => {
    setOpen(false)
    setCellBackground(editor, {
      color,
      selectedCells: editor.getApi(TablePlugin).table.getSelectedCells() ?? [],
    })
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen} modal={false}>
      <DropdownMenuTrigger render={<ToolbarButton tooltip={tooltip} />}>
        {children}
      </DropdownMenuTrigger>

      <DropdownMenuContent align="start">
        <ToolbarMenuGroup label="Colors">
          <div className="grid grid-cols-[repeat(10,1fr)] place-items-center gap-x-1 px-2">
            {TABLE_CELL_COLORS.map((color) => (
              <ColorSwatch key={color.name} {...color} onPick={paint} />
            ))}
          </div>
        </ToolbarMenuGroup>
        <DropdownMenuGroup>
          <DropdownMenuItem className="p-2" onClick={() => paint(null)}>
            <EraserIcon />
            <span>Clear</span>
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
