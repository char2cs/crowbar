import type { ToastTone } from './toast-tone'

export interface PaneToast {
  key: string
  tone: ToastTone
  title: string
  description?: string
  pinned: boolean
}
