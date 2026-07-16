// copied from Athas — no shadcn/ui equivalent
import { useEffect, useState, type ReactNode, type ChangeEvent } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

type ButtonVariant = 'default' | 'outline' | 'secondary' | 'ghost' | 'destructive' | 'link'

interface PrimitiveChoiceOption {
  value: string
  label: string
  variant?: ButtonVariant
}

type PrimitiveDialogRequest =
  | {
      id: number
      type: 'alert'
      title: string
      message: ReactNode
      resolve: () => void
    }
  | {
      id: number
      type: 'confirm'
      title: string
      message: ReactNode
      confirmLabel: string
      cancelLabel: string
      resolve: (value: boolean) => void
    }
  | {
      id: number
      type: 'choice'
      title: string
      message: ReactNode
      choices: PrimitiveChoiceOption[]
      resolve: (value: string | null) => void
    }
  | {
      id: number
      type: 'prompt'
      title: string
      message: ReactNode
      defaultValue: string
      placeholder?: string
      confirmLabel: string
      cancelLabel: string
      resolve: (value: string | null) => void
    }

interface PrimitiveConfirmOptions {
  title?: string
  confirmLabel?: string
  cancelLabel?: string
}

interface PrimitivePromptOptions extends PrimitiveConfirmOptions {
  defaultValue?: string
  placeholder?: string
}

let nextDialogId = 1
let enqueueDialog: ((request: PrimitiveDialogRequest) => void) | null = null
const pendingDialogs: PrimitiveDialogRequest[] = []

function enqueue(request: PrimitiveDialogRequest) {
  if (enqueueDialog) {
    enqueueDialog(request)
    return
  }

  pendingDialogs.push(request)
}

// react-doctor-disable-next-line only-export-components -- imperative dialog API sharing this module's dialog queue state (enqueue/nextDialogId) with PrimitiveDialogProvider below; the API and the provider that renders its queue are one unit.
export function primitiveAlert(message: ReactNode, title = 'Notice'): Promise<void> {
  return new Promise((resolve) => {
    enqueue({ id: nextDialogId++, type: 'alert', title, message, resolve })
  })
}

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue (see primitiveAlert).
export function primitiveConfirm(
  message: ReactNode,
  options: PrimitiveConfirmOptions = {},
): Promise<boolean> {
  return new Promise((resolve) => {
    enqueue({
      id: nextDialogId++,
      type: 'confirm',
      title: options.title ?? 'Confirm',
      message,
      confirmLabel: options.confirmLabel ?? 'Confirm',
      cancelLabel: options.cancelLabel ?? 'Cancel',
      resolve,
    })
  })
}

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue (see primitiveAlert).
export function primitivePrompt(
  message: ReactNode,
  options: PrimitivePromptOptions = {},
): Promise<string | null> {
  return new Promise((resolve) => {
    enqueue({
      id: nextDialogId++,
      type: 'prompt',
      title: options.title ?? 'Input',
      message,
      defaultValue: options.defaultValue ?? '',
      placeholder: options.placeholder,
      confirmLabel: options.confirmLabel ?? 'OK',
      cancelLabel: options.cancelLabel ?? 'Cancel',
      resolve,
    })
  })
}

export function PrimitiveDialogProvider({ children }: { children: ReactNode }) {
  const [queue, setQueue] = useState<PrimitiveDialogRequest[]>([])

  useEffect(() => {
    enqueueDialog = (request) => setQueue((current) => [...current, request])

    if (pendingDialogs.length > 0) {
      setQueue((current) => [...current, ...pendingDialogs.splice(0)])
    }

    return () => {
      enqueueDialog = null
    }
  }, [])

  const activeDialog = queue[0] ?? null
  const closeActive = (resolve: () => void) => {
    resolve()
    setQueue((current) => current.slice(1))
  }

  return (
    <>
      {children}
      {activeDialog && (
        <PrimitiveDialogHost key={activeDialog.id} dialog={activeDialog} onClose={closeActive} />
      )}
    </>
  )
}

function PrimitiveDialogHost({
  dialog,
  onClose,
}: {
  dialog: PrimitiveDialogRequest
  onClose: (resolve: () => void) => void
}) {
  const [promptValue, setPromptValue] = useState(
    dialog.type === 'prompt' ? dialog.defaultValue : '',
  )
  const [open, setOpen] = useState(true)

  const handleClose = (resolve: () => void) => {
    setOpen(false)
    onClose(resolve)
  }

  if (dialog.type === 'alert') {
    return (
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialog.title}</DialogTitle>
          </DialogHeader>
          <div className="whitespace-pre-wrap text-xs text-foreground">{dialog.message}</div>
          <DialogFooter>
            <Button onClick={() => handleClose(dialog.resolve)}>OK</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  if (dialog.type === 'confirm') {
    return (
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialog.title}</DialogTitle>
          </DialogHeader>
          <div className="whitespace-pre-wrap text-xs text-foreground">{dialog.message}</div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleClose(() => dialog.resolve(false))}>
              {dialog.cancelLabel}
            </Button>
            <Button onClick={() => handleClose(() => dialog.resolve(true))}>
              {dialog.confirmLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  if (dialog.type === 'choice') {
    return (
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialog.title}</DialogTitle>
          </DialogHeader>
          <div className="whitespace-pre-wrap text-xs text-foreground">{dialog.message}</div>
          <DialogFooter>
            {dialog.choices.map((choice) => (
              <Button
                key={choice.value}
                variant={choice.variant ?? 'default'}
                onClick={() => handleClose(() => dialog.resolve(choice.value))}
              >
                {choice.label}
              </Button>
            ))}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{dialog.title}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(event: ChangeEvent<HTMLFormElement> & React.FormEvent<HTMLFormElement>) => {
            event.preventDefault()
            handleClose(() => dialog.resolve(promptValue))
          }}
          className="flex flex-col gap-2"
        >
          <label className="flex flex-col gap-2 text-sm text-foreground">
            {dialog.message}
            <Input
              autoFocus
              value={promptValue}
              placeholder={dialog.placeholder}
              onChange={(event: ChangeEvent<HTMLInputElement>) =>
                setPromptValue(event.target.value)
              }
            />
          </label>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(() => dialog.resolve(null))}>
            {dialog.cancelLabel}
          </Button>
          <Button onClick={() => handleClose(() => dialog.resolve(promptValue))}>
            {dialog.confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
