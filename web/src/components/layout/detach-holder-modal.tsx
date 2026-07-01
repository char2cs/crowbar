import {
  Dialog,
  DialogPopup,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { detachHolder } from '@/lib/api/workspace'

// The consent modal for freeing a protected branch from a live holder. Framed as
// disruptive-not-destructive: per the git engine's contract, detach never touches
// the working tree — only moves HEAD — so uncommitted changes and commits are
// preserved (spec §3.7). A detach blocked mid-merge/rebase surfaces cleanly as a
// LastError from the backend (no partial state), which the toast/entity surface.
export function DetachHolderModal() {
  const target = useDetachModalStore((s) => s.target)
  const close = useDetachModalStore((s) => s.close)

  if (!target) return null

  const onConfirm = async () => {
    await detachHolder(target.wsId)
    close()
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogPopup className="max-w-md">
        <DialogHeader className="pr-10">
          <DialogTitle>Detach to manage {target.branch}</DialogTitle>
          <DialogDescription className="leading-relaxed">
            The checkout at <span className="font-mono text-foreground">{target.heldByPath}</span>{' '}
            will move to a detached HEAD, releasing{' '}
            <span className="font-mono text-foreground">{target.branch}</span> so Crowbar can manage
            it in its own worktree. Your files are safe — only the working directory's current
            branch changes; uncommitted changes and commits are preserved.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button onClick={onConfirm}>Detach</Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
