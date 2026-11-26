import { useRef } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface DialogRequest {
  id: number
  title: string
  message: ReactNode
  confirmLabel: string
  cancelLabel: string
  destructive: boolean
  onConfirm: () => void
  onDismiss: () => void
}

export interface DialogHostProps {
  request: DialogRequest
  onSettled: () => void
}

export function DialogHost({ request, onSettled }: DialogHostProps) {
  // Closing the dialog makes Base UI emit onOpenChange(false) as it unmounts, which would
  // otherwise settle the request a second time and drop the next one off the queue.
  const settled = useRef(false)
  // Focus starts on the safe answer, so a stray Enter never confirms.
  const cancelRef = useRef<HTMLButtonElement>(null)

  const settle = (notifyCaller: () => void) => {
    if (settled.current) return
    settled.current = true
    notifyCaller()
    onSettled()
  }

  const dismiss = () => settle(request.onDismiss)
  const confirm = () => settle(request.onConfirm)

  return (
    <Dialog open onOpenChange={(next) => !next && dismiss()}>
      <DialogContent initialFocus={cancelRef}>
        <DialogHeader>
          <DialogTitle>{request.title}</DialogTitle>
          {/* A div, not Base UI's <p>: the message is a ReactNode and may hold block elements. */}
          <DialogDescription render={<div />} className="whitespace-pre-wrap">
            {request.message}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button ref={cancelRef} variant="outline" onClick={dismiss}>
            {request.cancelLabel}
          </Button>
          <Button variant={request.destructive ? 'destructive' : 'default'} onClick={confirm}>
            {request.confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
