import { useEffect, useEffectEvent } from 'react'
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
import { hasNativeChrome, nativeConfirm } from '@/lib/native-chrome'
import { runOperation } from '@/core/entities/run-operation'

type Target = NonNullable<ReturnType<typeof useDetachModalStore.getState>['target']>

function detach(target: Target) {
  runOperation(
    {
      kind: 'workspace.detach_holder',
      subject: { kind: 'workspace', id: target.wsId },
      projectId: '',
      label: `Detach ${target.heldByPath}`,
    },
    (opId) => detachHolder(target.wsId, opId),
  )
}

function describeDetach(target: Target): string {
  return `The checkout at ${target.heldByPath} will move to a detached HEAD, releasing ${target.branch} so Crowbar can manage it in its own worktree. Your files are safe — only the working directory's current branch changes; uncommitted changes and commits are preserved.`
}

// The phone asks the same question in a native alert, over whichever sheet is on screen.
function NativeDetachConfirm({ target, close }: { target: Target; close: () => void }) {
  const answer = useEffectEvent((yes: boolean) => {
    if (yes) detach(target)
    close()
  })
  useEffect(() => {
    let live = true
    void nativeConfirm({
      title: `Detach to manage ${target.branch}`,
      message: describeDetach(target),
      confirmLabel: 'Detach',
      cancelLabel: 'Cancel',
      destructive: false,
    }).then((yes) => {
      if (live) answer(yes)
    })
    return () => {
      live = false
    }
  }, [target])
  return null
}

// Consent to free a protected branch from a live holder: disruptive, not destructive — detach only
// moves HEAD, so work is preserved (§3.7). A detach blocked mid-merge fails its operation.
export function DetachHolderModal() {
  const target = useDetachModalStore((s) => s.target)
  const close = useDetachModalStore((s) => s.close)

  if (!target) return null
  if (hasNativeChrome()) return <NativeDetachConfirm target={target} close={close} />

  const onConfirm = () => {
    detach(target)
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
