import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { FileIcon, PencilIcon, PlusIcon } from '@/features/agent/shared/agent-icons'

interface ComposerPlusButtonProps {
  onOpenExcalidraw: () => void
  onOpenAttachFile: () => void
}

/** Styled neutrally: it opens a choice and sends nothing, so it never borrows
 *  send's primary treatment. */
export function ComposerPlusButton({
  onOpenExcalidraw,
  onOpenAttachFile,
}: ComposerPlusButtonProps) {
  const [open, setOpen] = useState(false)

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        // `sm:size-7` overrides icon-xs's `sm:size-6` so plus matches send's 28px
        // (PLUS_DIAMETER === SEND_DIAMETER in handle-geometry.ts).
        className="plusbtn rounded-full sm:size-7"
        render={<Button variant="ghost" size="icon-xs" />}
        data-open={open || undefined}
        aria-label="Add to this message"
        title="Add to this message"
      >
        <PlusIcon size={16} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" sideOffset={8}>
        <DropdownMenuItem onClick={onOpenExcalidraw}>
          <PencilIcon size={14} />
          <span>Excalidraw</span>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onOpenAttachFile}>
          <FileIcon size={14} />
          <span>Attach File</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
