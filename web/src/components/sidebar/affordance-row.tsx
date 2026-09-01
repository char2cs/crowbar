import { ChatsCircle } from '@phosphor-icons/react'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'

interface AffordanceRowProps {
  onCreateThread: () => void
  onCreateWorkspace?: () => void
}

/**
 * The affordance row for empty containers — the only way to create a thread
 * (or workspace, when git-capable). Icon-only, always visible (ROW_SUB_ACTION,
 * not the *_HOVER variant — this icon IS the row's content, not a secondary
 * trailing control); the dropdown menu itself appears on click only, when
 * onCreateWorkspace is provided.
 */
export function AffordanceRow({ onCreateThread, onCreateWorkspace }: AffordanceRowProps) {
  if (!onCreateWorkspace) {
    return (
      <button
        type="button"
        className={ROW_SUB_ACTION}
        aria-label="Create new thread"
        onClick={(e) => {
          e.stopPropagation()
          onCreateThread()
        }}
      >
        <ChatsCircle aria-hidden="true" className="size-3" weight="regular" />
      </button>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        data-testid="affordance-dropdown"
        aria-label="Create new thread or workspace"
        className={ROW_SUB_ACTION}
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        <ChatsCircle aria-hidden="true" className="size-3" weight="regular" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="bottom" sideOffset={4}>
        <DropdownMenuItem onClick={onCreateThread}>Create thread</DropdownMenuItem>
        <DropdownMenuItem onClick={onCreateWorkspace}>Create workspace</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
