import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

/**
 * Promise-based replacements for window.alert/confirm/prompt, rendered with the app's
 * own Dialog so they inherit theming and cannot block the main thread.
 *
 * Requests queue up and are shown one at a time. Every request carries a `dismiss`
 * outcome, so closing a dialog by any route — Cancel, Escape, or a backdrop click —
 * settles the caller's promise instead of leaving it pending forever.
 */

interface ConfirmOptions {
  title?: string
  confirmLabel?: string
  cancelLabel?: string
}

interface PromptOptions extends ConfirmOptions {
  defaultValue?: string
  placeholder?: string
}

type DialogRequest = { id: number; title: string; message: ReactNode } & (
  | { kind: 'alert'; confirmLabel: string; onConfirm: () => void; onDismiss: () => void }
  | {
      kind: 'confirm'
      confirmLabel: string
      cancelLabel: string
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

let nextId = 1
let deliver: ((request: DialogRequest) => void) | null = null
// Requests raised before the provider mounts wait here rather than being dropped.
const backlog: DialogRequest[] = []

function enqueue(request: DialogRequest) {
  if (deliver) {
    deliver(request)
  } else {
    backlog.push(request)
  }
}

export function primitiveAlert(message: ReactNode, title = 'Notice'): Promise<void> {
  return new Promise((resolve) => {
    enqueue({
      id: nextId++,
      kind: 'alert',
      title,
      message,
      confirmLabel: 'OK',
      onConfirm: () => resolve(),
      onDismiss: () => resolve(),
    })
  })
}

export function primitiveConfirm(
  message: ReactNode,
  options: ConfirmOptions = {},
): Promise<boolean> {
  return new Promise((resolve) => {
    enqueue({
      id: nextId++,
      kind: 'confirm',
      title: options.title ?? 'Confirm',
      message,
      confirmLabel: options.confirmLabel ?? 'Confirm',
      cancelLabel: options.cancelLabel ?? 'Cancel',
      onConfirm: () => resolve(true),
      onDismiss: () => resolve(false),
    })
  })
}

export function primitivePrompt(
  message: ReactNode,
  options: PromptOptions = {},
): Promise<string | null> {
  return new Promise((resolve) => {
    enqueue({
      id: nextId++,
      kind: 'prompt',
      title: options.title ?? 'Input',
      message,
      defaultValue: options.defaultValue ?? '',
      placeholder: options.placeholder,
      confirmLabel: options.confirmLabel ?? 'OK',
      cancelLabel: options.cancelLabel ?? 'Cancel',
      onConfirm: (value) => resolve(value),
      onDismiss: () => resolve(null),
    })
  })
}

export function PrimitiveDialogProvider({ children }: { children: ReactNode }) {
  const [queue, setQueue] = useState<DialogRequest[]>([])

  useEffect(() => {
    deliver = (request) => setQueue((current) => [...current, request])

    if (backlog.length > 0) {
      setQueue((current) => [...current, ...backlog.splice(0)])
    }

    return () => {
      deliver = null
    }
  }, [])

  const dequeue = useCallback(() => setQueue((current) => current.slice(1)), [])

  const active = queue[0]

  return (
    <>
      {children}
      {active ? <DialogHost key={active.id} request={active} onSettled={dequeue} /> : null}
    </>
  )
}

function DialogHost({ request, onSettled }: { request: DialogRequest; onSettled: () => void }) {
  const [value, setValue] = useState(request.kind === 'prompt' ? request.defaultValue : '')
  // Closing the dialog makes Base UI emit onOpenChange(false) as it unmounts, which would
  // otherwise settle the request a second time and drop the next one off the queue.
  const settled = useRef(false)

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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{request.title}</DialogTitle>
        </DialogHeader>

        {request.kind === 'prompt' ? (
          <form
            className="flex flex-col gap-2"
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
        ) : (
          <div className="whitespace-pre-wrap text-xs text-foreground">{request.message}</div>
        )}

        <DialogFooter>
          {request.kind !== 'alert' && (
            <Button variant="outline" onClick={dismiss}>
              {request.cancelLabel}
            </Button>
          )}
          <Button onClick={confirm}>{request.confirmLabel}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
