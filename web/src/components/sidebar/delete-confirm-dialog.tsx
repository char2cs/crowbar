import { useEffect, useState } from 'react'
import { Dialog, DialogPopup, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { fetchDeletePreview, type DeletePreview } from '@/components/sidebar/lib/delete-preview-client'

interface DeleteConfirmDialogProps {
  /** Names the row a trash click just targeted. Mounted once and reused
   *  across rows exactly like `RenameDialog` — this flag, not a locally
   *  tracked id, is what opens it. */
  open: boolean
  /** The row's own label, named in both the confirm copy and the refusal. */
  label: string
  /** Spec §9: "A working chat is not deletable. REFUSED, not confirmed."
   *  This is a fast local mirror of the backend's own `guardNotWorking` —
   *  the backend addendum spec (§2) keeps that the authoritative check, this
   *  just avoids a round trip for the common case. */
  working: boolean
  /** Undefined until the row's repo has seeded a `projectId` — the preview
   *  fetch is skipped rather than sent with a garbage URL segment. */
  projectId: string | undefined
  repoId: string
  chatId: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

/**
 * The sidebar's one delete-confirm gesture (spec §9). A working row never
 * reaches the `Dialog` at all — the backend addendum's invariant 9 makes
 * that refusal unconditional and not confirmable past, so rendering it as a
 * real dialog with a Cancel/Delete pair would imply a choice that does not
 * exist. An idle row's dialog names what the delete takes: the file count
 * comes from `delete-preview-client.ts`, the chat count rides along with it
 * (backend addendum spec §1 sums both in one call).
 */
export function DeleteConfirmDialog({
  open,
  label,
  working,
  projectId,
  repoId,
  chatId,
  onOpenChange,
  onConfirm,
}: DeleteConfirmDialogProps) {
  // Freezes the refuse-vs-confirm call for THIS click the moment it starts,
  // rather than re-deriving it from the live `working` prop on every render.
  //
  // Spec §9: "REFUSED, not confirmed" has to be terminal for the click that
  // triggered it. Reading `working` live meant a turn finishing while the
  // refusal message was still on screen silently matured it into a REAL
  // delete-confirmation dialog on the next render, with zero new user
  // action (review round 1, Critical).
  //
  // Keyed on `chatId` changing, not just `open` going true: the refusal
  // branch below renders no backdrop, so the tree stays fully clickable —
  // a second trash click on a DIFFERENT row can retarget this same
  // already-open dialog without `open` ever cycling back through `false`,
  // and that retarget must start its own fresh decision, not inherit the
  // first row's. This is the render-time "adjust state from props" pattern
  // (same shape as React's own docs on the topic): it re-derives `decision`
  // synchronously within render, so there is no frame where a stale value
  // could paint.
  const [decision, setDecision] = useState<{ chatId: string; working: boolean } | null>(null)
  if (open && (decision === null || decision.chatId !== chatId)) {
    setDecision({ chatId, working })
  } else if (!open && decision !== null) {
    setDecision(null)
  }
  const frozenWorking = decision?.working ?? working

  const [preview, setPreview] = useState<DeletePreview | null>(null)
  const [previewFailed, setPreviewFailed] = useState(false)

  useEffect(() => {
    if (!open || frozenWorking) {
      setPreview(null)
      setPreviewFailed(false)
      return
    }
    if (!projectId) {
      setPreviewFailed(true)
      return
    }
    let cancelled = false
    fetchDeletePreview(projectId, repoId, chatId)
      .then((result) => {
        if (!cancelled) setPreview(result)
      })
      .catch(() => {
        if (!cancelled) setPreviewFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [open, frozenWorking, projectId, repoId, chatId])

  // No Dialog primitive at all here — not even a closed one — so a working
  // row can never surface `role="dialog"`, matching the unconditional refusal.
  if (open && frozenWorking) {
    return (
      <div role="status" className="px-3 py-1.5 text-destructive text-xs">
        {label} is still working — delete refuses until the turn finishes.
      </div>
    )
  }

  return (
    <Dialog open={open && !frozenWorking} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Delete {label}?</DialogTitle>
        </DialogHeader>
        <div className="px-6 pb-2 text-muted-foreground text-sm">
          {preview ? (
            <>
              This takes {label} and everything under it — {preview.fileCount} uncommitted files
              and {preview.chatCount} chats.
            </>
          ) : previewFailed ? (
            <>This takes {label} and everything under it.</>
          ) : (
            <>Checking what this takes…</>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => {
              onConfirm()
              onOpenChange(false)
            }}
          >
            Delete
          </Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
