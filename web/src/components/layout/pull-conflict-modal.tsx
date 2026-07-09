import {
  Dialog,
  DialogPopup,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { usePullConflictModalStore } from '@/features/git/stores/use-pull-conflict-modal-store'

// Inform-only modal shown when Pull is refused because the branch has diverged
// from its upstream and can't fast-forward (backend 409 `not_fast_forwardable`).
// Crowbar never merges two divergent histories on its own — that could conflict
// and wedge the working tree — so it stops and explains. There is no action here
// beyond acknowledging; the user integrates the upstream (rebase/merge) and pulls
// again. Mirrors detach-holder-modal.tsx exactly (strict cossui Dialog, tokens
// only, single primary button).
export function PullConflictModal() {
  const target = usePullConflictModalStore((s) => s.target)
  const close = usePullConflictModalStore((s) => s.close)

  if (!target) return null

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogPopup className="max-w-md">
        <DialogHeader className="pr-10">
          <DialogTitle>Can't pull — branch has diverged</DialogTitle>
          <DialogDescription className="leading-relaxed">
            <span className="font-mono text-foreground">{target.branch}</span> has local commits
            that origin doesn't, so it can't fast-forward. Crowbar won't merge the two histories
            automatically — that could create conflicts and leave your working tree wedged.
            Integrate the upstream changes yourself (rebase or merge), then pull again.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={close}>Got it</Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
