import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'
import type { DialogHostProps, DialogRequest } from './primitive-dialog-host'
import { hasNativeChrome, nativeConfirm } from '@/lib/native-chrome'

// Promise-based confirm/prompt in the app's own Dialog, shown one at a time. Closing one
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

// The dialog UI is not needed for first paint, so it loads with the first request.
const DialogHost = lazy<ComponentType<DialogHostProps>>(() =>
  import('./primitive-dialog-host').then(
    (module) => ({ default: module.DialogHost }),
    () => ({ default: DismissOnMount }),
  ),
)

// The host failed to load: this provider sits above every error boundary, so settle the
// request as dismissed rather than throw or leave it pending.
function DismissOnMount({ request, onSettled }: DialogHostProps) {
  const settled = useRef(false)
  useEffect(() => {
    if (settled.current) return
    settled.current = true
    request.onDismiss()
    onSettled()
  }, [request, onSettled])
  return null
}

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

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue.
export function primitiveConfirm(
  message: ReactNode,
  options: ConfirmOptions = {},
): Promise<boolean> {
  // The phone's sheets cover this webview; a native alert shows over them.
  if (hasNativeChrome() && typeof message === 'string') {
    return nativeConfirm({
      title: options.title ?? 'Confirm',
      message,
      confirmLabel: options.confirmLabel ?? 'Confirm',
      cancelLabel: options.cancelLabel ?? 'Cancel',
      destructive: options.destructive ?? false,
    })
  }
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

// react-doctor-disable-next-line only-export-components -- imperative dialog API backed by this module's shared queue (see primitiveConfirm).
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
      {active ? (
        <Suspense fallback={null}>
          <DialogHost key={active.id} request={active} onSettled={dequeue} />
        </Suspense>
      ) : null}
    </>
  )
}
