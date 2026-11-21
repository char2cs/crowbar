import type { ToastTone } from './toast-tone'

/** A toast a pane asks the shell to show; the same `key` rewrites it in place. */
export interface PaneToast {
  key: string
  tone: ToastTone
  title: string
  description?: string
  pinned: boolean
}
