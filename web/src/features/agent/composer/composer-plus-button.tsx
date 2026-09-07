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

/**
 * The composer's second control, styled neutrally — it opens a choice, it
 * does not send anything, so it never borrows send's primary treatment.
 *
 * Two entries only, per the design spec: Excalidraw and Attach File are
 * deliberately NOT the same entry point, because Excalidraw opens an editor
 * (produces a scene) while Attach File opens a picker (ingests an existing
 * file) — kind for a picked file is inferred from the file itself, kind for
 * this entry is fixed by which one was clicked.
 */
export function ComposerPlusButton({
  onOpenExcalidraw,
  onOpenAttachFile,
}: ComposerPlusButtonProps) {
  const [open, setOpen] = useState(false)

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        // `icon-xs` shrinks to `sm:size-6` (24px) at the `sm:` breakpoint —
        // this app's window is always past that, so it rendered visibly
        // smaller than `.send`'s fixed 28px. `sm:size-7` beats it (same
        // conflict group, same modifier) and matches PLUS_DIAMETER ===
        // SEND_DIAMETER (handle-geometry.ts) — the two must read as one
        // circle size, not two.
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
