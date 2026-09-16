import { useEffect, useState } from 'react'
import {
  Dialog,
  DialogPopup,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

interface RenameDialogProps {
  open: boolean
  initialValue: string
  onOpenChange: (open: boolean) => void
  onConfirm: (name: string) => void
}

/**
 * The sidebar's one rename gesture, replacing the deleted tree's inline
 * double-click editor with a modal — this task's context menu offers Rename
 * on every row kind, and a modal (rather than an inline input drawn per row)
 * keeps `SidebarRow` itself dumb (spec §3.3: no second line, no per-row edit
 * state).
 */
export function RenameDialog({ open, initialValue, onOpenChange, onConfirm }: RenameDialogProps) {
  const [value, setValue] = useState(initialValue)

  // Reseed on every open — this dialog stays mounted across rows, so without
  // this the second row renamed would start from the first row's leftover text.
  useEffect(() => {
    if (open) setValue(initialValue)
  }, [open, initialValue])

  function handleRename() {
    const trimmed = value.trim()
    if (!trimmed) return
    onConfirm(trimmed)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename</DialogTitle>
        </DialogHeader>
        <div className="px-6 pb-2">
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                handleRename()
              }
            }}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleRename}>Rename</Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
