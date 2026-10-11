import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
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

// Promise-based alert/confirm/prompt in the app's own Dialog, shown one at a time. Closing one
// by any route (Cancel, Escape, the backdrop) settles its promise: none is left pending.

interface DialogLabels {
  title?: string
  confirmLabel?: string
  cancelLabel?: string
}

interface ConfirmOptions extends DialogLabels {
  /** The confirm button reads as destructive. Focus starts on cancel whatever this says. */
  destructive?: boolean
}

interface PromptOptions extends DialogLabels {
  defaultValue?: string
  placeholder?: string
}

type DialogRequest = { id: number; title: string; message: ReactNode } & (
  | { kind: 'alert'; confirmLabel: string; onConfirm: () => void; onDismiss: () => void }
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

// react-doctor-disable-next-line only-export-components -- imperative dialog API sharing this module's dialog queue state (enqueue/nextId) with PrimitiveDialogProvider below; the API and the provider that renders its queue are one unit.
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

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue (see primitiveAlert).
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
      destructive: options.destructive ?? false,
      onConfirm: () => resolve(true),
      onDismiss: () => resolve(false),
    })
  })
}

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue (see primitiveAlert).
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
  // A confirm starts on its safe answer, so a stray Enter never confirms.
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
          {/* A div, not Base UI's <p>: the message is a ReactNode and may hold block elements. */}
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
          {request.kind !== 'alert' && (
            <Button ref={cancelRef} variant="outline" onClick={dismiss}>
              {request.cancelLabel}
            </Button>
          )}
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
