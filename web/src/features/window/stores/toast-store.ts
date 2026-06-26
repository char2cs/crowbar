// web/src/features/window/stores/toast-store.ts
// Thin imperative toast API backed by CossUI toastManager.
// No Zustand, no sonner — just delegates to @base-ui/react/toast via the
// generated component wrapper.
import { toastManager } from '@/components/ui/toast'

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
  show: (opts: ShowOptions): string => toastManager.add(toAdd(opts)) as string,
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
