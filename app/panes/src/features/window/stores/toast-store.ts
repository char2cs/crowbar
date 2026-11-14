// Thin imperative toast API over base-ui's toastManager; no store of its own.
import { toastManager } from '@/lib/toast-manager'

interface ShowOptions {
  message: string
  description?: string
  type?: 'info' | 'success' | 'warning' | 'error'
  key?: string
  duration?: number
  action?: { label: string; onClick: () => void }
}

function toAdd(opts: ShowOptions) {
  return {
    title: opts.message,
    description: opts.description,
    type: opts.type,
    id: opts.key,
    timeout: opts.duration,
    actionProps: opts.action
      ? { children: opts.action.label, onClick: opts.action.onClick }
      : undefined,
  }
}

export const toast = {
  show: (opts: ShowOptions): string => toastManager.add(toAdd(opts)),
  dismiss: (id: string) => toastManager.close(id),
  dismissByKey: (key: string) => toastManager.close(key),
  info: (message: string, description?: string) =>
    toastManager.add({ title: message, description, type: 'info' }),
  success: (message: string, description?: string) =>
    toastManager.add({ title: message, description, type: 'success' }),
  warning: (message: string, description?: string) =>
    toastManager.add({ title: message, description, type: 'warning' }),
  error: (message: string, description?: string) =>
    toastManager.add({ title: message, description, type: 'error' }),
}
