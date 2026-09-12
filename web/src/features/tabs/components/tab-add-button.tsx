// Lucide Plus (stroke-based) to match the sibling toolbar icons — back/forward/
// settings are Lucide; Phosphor's bold Plus rendered heavier and larger than them.
import { Plus } from 'lucide-react'
import { FilePlus, TerminalWindow } from '@phosphor-icons/react'
import React from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface TabAddButtonProps {
  isBottomPane: boolean
  onNewFile: () => void
  onNewTerminal: () => void
}

/**
 * The tab strip's "+" — a dropdown offering the two things that can land in
 * this pane's editor view: a new (blank, unsaved) file, or a new terminal.
 * It is rendered as the LAST child inside the scrolling tab container (see
 * tab-bar.tsx), immediately after the last tab, so it flows and shifts
 * horizontally as tabs open and close — the same placement a browser's
 * new-tab button uses. It is a plain button, never wrapped in
 * `SortableEditorTab`/`useSortable`, so it never enters `sortedBufferIds`
 * and is never draggable.
 */
const TabAddButton = React.memo(function TabAddButton({
  isBottomPane,
  onNewFile,
  onNewTerminal,
}: TabAddButtonProps) {
  if (isBottomPane) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="New tab"
        className="inline-flex size-8 shrink-0 items-center justify-center rounded-sm text-muted-foreground outline-none hover:bg-sidebar-element-hover data-[popup-open]:bg-sidebar-element-hover sm:size-7"
      >
        <Plus className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-40">
        <DropdownMenuItem onClick={onNewFile}>
          <FilePlus className="size-4" />
          New File
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onNewTerminal}>
          <TerminalWindow className="size-4" />
          New Terminal
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
})

export default TabAddButton
