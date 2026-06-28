import { Chat, Plus, TerminalWindow as Terminal, X } from '@phosphor-icons/react'
import React from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'

interface TabNewButtonProps {
  isBottomPane: boolean
  disablePaneActions: boolean
  isInSplit: boolean
  onNewConversation: () => void
  onNewTerminal: () => void
  onClosePane: () => void
}

const TabNewButton = React.memo(function TabNewButton({
  isBottomPane,
  disablePaneActions,
  isInSplit,
  onNewConversation,
  onNewTerminal,
  onClosePane,
}: TabNewButtonProps) {
  if (isBottomPane) return null

  return (
    <div className="flex shrink-0 items-center gap-1 pl-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-xs"
              className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
              aria-label="New tab"
            />
          }
        >
          <Plus weight="bold" size={12} />
        </DropdownMenuTrigger>
        <DropdownMenuContent side="bottom" align="start" className="min-w-[160px]">
          <DropdownMenuItem onClick={onNewConversation}>
            <Chat className="text-muted-foreground" />
            New Conversation
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onNewTerminal}>
            <Terminal className="text-muted-foreground" />
            New Terminal
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {!disablePaneActions && isInSplit && (
        <Button
          onClick={onClosePane}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
          tooltip="Close Split"
          tooltipSide="bottom"
          aria-label="Close split pane"
        >
          <X size={12} />
        </Button>
      )}
    </div>
  )
})

export default TabNewButton
