import { toastManager } from '@/lib/toast-manager'
import type { PaneClient } from '@/sdk/pane-client'
import type { ToastTone } from '@/sdk/toast-tone'

type Shell = Pick<PaneClient, 'showToast' | 'closeToast'>

interface Shown {
  id: string
  type?: string
  title?: unknown
  description?: unknown
  timeout?: number
}

type Updates = Partial<Shown> | ((previous: Shown) => Partial<Shown>)

type Stage = string | Partial<Shown> | ((value: unknown) => string | Partial<Shown>)

interface Promised {
  promise: Promise<unknown>
  loading: Stage
  success: Stage
  error: Stage
}

const TONES: readonly ToastTone[] = ['info', 'success', 'warning', 'error', 'loading']

const toneOf = (type: string | undefined): ToastTone =>
  TONES.find((tone) => tone === type) ?? 'info'

const textOf = (node: unknown): string => (typeof node === 'string' ? node : '')

function stageOf(stage: Stage, type: ToastTone, value?: unknown): Partial<Shown> {
  const options = typeof stage === 'function' ? stage(value) : stage
  return typeof options === 'string' ? { title: options, type } : { type, ...options }
}

function show(shell: Shell, toast: Shown): void {
  const title = textOf(toast.title)
  const description = textOf(toast.description)
  const headline = title || description
  if (!headline) return
  shell.showToast({
    key: toast.id,
    tone: toneOf(toast.type),
    title: headline,
    ...(title && description ? { description } : {}),
    pinned: toast.timeout === 0,
  })
}

/** Shows every `toastManager` toast in the shell's toast host, where develop's one viewport (the sidebar column) sits; it is the pane's only listener, so toasts raised before it started are shown too. Returns the stop. */
export function forwardToastsToShell(shell: Shell): () => void {
  const shown = new Map<string, Shown>()
  const put = (toast: Shown) => {
    shown.set(toast.id, toast)
    show(shell, toast)
  }
  const closeOne = (key: string) => {
    shown.delete(key)
    shell.closeToast(key)
  }
  const close = (id: string | undefined) =>
    (id === undefined ? [...shown.keys()] : [id]).forEach(closeOne)
  const update = (id: string, updates: Updates) => {
    const previous = shown.get(id)
    if (!previous) return
    const next = typeof updates === 'function' ? updates(previous) : updates
    put({ ...previous, ...next, id })
  }
  const promised = ({ promise, loading, success, error }: Promised) => {
    const id = crypto.randomUUID()
    put({ ...stageOf(loading, 'loading'), id, timeout: 0 })
    promise.then(
      (value) => put({ ...stageOf(success, 'success', value), id }),
      (reason: unknown) => put({ ...stageOf(error, 'error', reason), id }),
    )
  }
  return toastManager[' subscribe'](({ action, options }) => {
    if (action === 'add') put(options as Shown)
    if (action === 'promise') promised(options as Promised)
    if (action === 'update') update(options.id as string, options.updates as Updates)
    if (action === 'close') close(options.id as string | undefined)
  })
}
