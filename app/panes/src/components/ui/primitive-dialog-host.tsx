import { useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

export type DialogRequest = { id: number; title: string; message: ReactNode } & (
  | {
      kind: 'confirm'
      confirmLabel: string
      cancelLabel: string
      destructive: boolean
      onConfirm: () => void
      onDismiss: () => void
    }
  | {
      kind: 'prompt'
      confirmLabel: string
      cancelLabel: string
      defaultValue: string
      placeholder?: string
      onConfirm: (value: string) => void
      onDismiss: () => void
    }
)

export interface DialogHostProps {
  request: DialogRequest
  onSettled: () => void
}

export function DialogHost({ request, onSettled }: DialogHostProps) {
  const [value, setValue] = useState(request.kind === 'prompt' ? request.defaultValue : '')
  const settled = useRef(false)
  const cancelRef = useRef<HTMLButtonElement>(null)

  const settle = (notifyCaller: () => void) => {
    if (settled.current) return
    settled.current = true
    notifyCaller()
    onSettled()
  }

  const dismiss = () => settle(request.onDismiss)

  const confirm = () => {
    if (request.kind === 'prompt') {
      settle(() => request.onConfirm(value))
    } else {
      settle(request.onConfirm)
    }
  }

  return (
    <Dialog open onOpenChange={(next) => !next && dismiss()}>
      <DialogContent initialFocus={request.kind === 'confirm' ? cancelRef : undefined}>
        <DialogHeader>
          <DialogTitle>{request.title}</DialogTitle>
          {request.kind !== 'prompt' && (
            <DialogDescription render={<div />} className="whitespace-pre-wrap">
              {request.message}
            </DialogDescription>
          )}
        </DialogHeader>

        {request.kind === 'prompt' && (
          <form
            className="flex flex-col gap-2 px-6 pb-6"
            onSubmit={(event) => {
              event.preventDefault()
              confirm()
            }}
          >
            <label className="flex flex-col gap-2 text-sm text-foreground">
              {request.message}
              <Input
                autoFocus
                value={value}
                placeholder={request.placeholder}
                onChange={(event) => setValue(event.target.value)}
              />
            </label>
          </form>
        )}

        <DialogFooter>
          <Button ref={cancelRef} variant="outline" onClick={dismiss}>
            {request.cancelLabel}
          </Button>
          <Button
            variant={request.kind === 'confirm' && request.destructive ? 'destructive' : 'default'}
            onClick={confirm}
          >
            {request.confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
